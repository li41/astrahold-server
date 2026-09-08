package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const maxPendingClassMessagesPerSession = 8

var ErrInitialClassSelectionFeedbackBacklog = errors.New("worldruntime: initial class selection feedback backlog full")

type initialClassSelectionFeedback struct {
	ownership            SessionOwnershipFence
	clientActionSequence uint32
	target               classid.ID
}

// EnqueueInitialClassSelection is the unfenced gateway seam used by ephemeral development adapters.
// World-owner validation still rejects any durable assignment without a trusted character identity.
func (r *Runtime) EnqueueInitialClassSelection(id session.ID, sequence uint32, intent protocol.ClientInitialClassSelection) error {
	if id == 0 || sequence == 0 || strings.TrimSpace(intent.ClassID) == "" {
		return errors.New("worldruntime: invalid initial class selection intent")
	}
	return r.queue.tryPush(initialClassAssignmentCommand{
		sessionID:       id,
		sequence:        sequence,
		target:          classid.ID(intent.ClassID),
		protocolRequest: true,
	})
}

// EnqueueFencedInitialClassSelection is the production network seam. The Client supplies only the
// requested ClassID; the trusted adapter supplies the ownership fence established by admission.
func (r *Runtime) EnqueueFencedInitialClassSelection(ownership SessionOwnershipFence, sequence uint32, intent protocol.ClientInitialClassSelection) error {
	if !ownership.Valid() || sequence == 0 || strings.TrimSpace(intent.ClassID) == "" {
		return ErrCharacterOwnershipFenceInvalid
	}
	return r.queue.tryPush(initialClassAssignmentCommand{
		sessionID:       ownership.SessionID,
		ownership:       ownership,
		sequence:        sequence,
		target:          classid.ID(intent.ClassID),
		protocolRequest: true,
	})
}

func (r *Runtime) guardInitialClassSelectionFeedbackCapacity(name string, s *session.Session, report *StepReport) bool {
	if s == nil || report == nil {
		return false
	}
	// Preserve two FIFO slots for a durable completion's authoritative state + committed result,
	// plus one slot for the current request if it is rejected immediately. This prevents later
	// assignment_pending rejections from consuming the capacity promised to an earlier selection.
	if len(r.pendingClassMessages[s.ID])+3 <= maxPendingClassMessagesPerSession {
		return true
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: s.ID, Err: ErrInitialClassSelectionFeedbackBacklog})
	if err := s.Connection().Close(); err != nil {
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: protocol.DeliveryReliableOrdered, MessageType: protocol.MessageInitialClassSelectionResult, Err: err})
	}
	return false
}

func (r *Runtime) rejectInitialClassSelection(name string, s *session.Session, clientActionSequence uint32, err error, report *StepReport) {
	if s == nil || report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: s.ID, Err: err})
	r.sendClassMessage(s, protocol.InitialClassSelectionResult{
		ClientActionSequence: clientActionSequence,
		ClassID:              r.authoritativeClassID(s),
		Outcome:              protocol.InitialClassSelectionRejected,
		Reason:               initialClassSelectionRejectionReason(err),
	}, report)
}

func (r *Runtime) authoritativeClassID(s *session.Session) string {
	if s == nil {
		return ""
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		return ""
	}
	return string(state.ClassID)
}

func (r *Runtime) queueCurrentClassState(s *session.Session) {
	if s == nil {
		return
	}
	pending := r.pendingClassMessages[s.ID]
	if len(pending) >= maxPendingClassMessagesPerSession {
		_ = s.Connection().Close()
		return
	}
	r.pendingClassMessages[s.ID] = append(pending, protocol.CharacterClassState{ClassID: r.authoritativeClassID(s)})
}

// publishDurableInitialClassSelection emits authoritative class state to the current owner after
// world-owner commit. The correlated result is emitted only if the original ownership fence is
// still current, so takeover never receives another connection's result.
func (r *Runtime) publishDurableInitialClassSelection(intentID uint64, identity characteridentity.Binding, target classid.ID, report *StepReport) {
	if report == nil || !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return
	}
	current, err := r.characterIdentities.currentOwnership(identity)
	if err != nil {
		return
	}
	s, ok := r.sessions.Get(current.SessionID)
	if !ok {
		return
	}
	state, ok := r.characters.State(current.EntityID)
	if !ok || state.ClassID != target {
		return
	}

	// State is truth and is deliberately ordered before any request-correlation result.
	r.sendClassMessage(s, protocol.CharacterClassState{ClassID: string(target)}, report)
	feedback, hasFeedback := r.initialClassSelectionFeedback[intentID]
	if hasFeedback && current == feedback.ownership {
		r.sendClassMessage(s, protocol.InitialClassSelectionResult{
			ClientActionSequence: feedback.clientActionSequence,
			ClassID:              string(target),
			Outcome:              protocol.InitialClassSelectionCommitted,
		}, report)
	}
}

func (r *Runtime) sendClassMessage(s *session.Session, message protocol.Message, report *StepReport) {
	if s == nil || message == nil || report == nil {
		return
	}
	pending := r.pendingClassMessages[s.ID]
	if len(pending) > 0 {
		if len(pending) >= maxPendingClassMessagesPerSession {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "class_feedback", SessionID: s.ID, Err: ErrInitialClassSelectionFeedbackBacklog})
			_ = s.Connection().Close()
			return
		}
		r.pendingClassMessages[s.ID] = append(pending, message)
		return
	}
	if err := r.trySendClassMessage(s, message, report.Tick, report); err != nil {
		if errors.Is(err, session.ErrBackpressure) {
			r.pendingClassMessages[s.ID] = append(r.pendingClassMessages[s.ID], message)
			return
		}
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: protocol.DeliveryReliableOrdered, MessageType: message.Type(), Err: err})
	}
}

func (r *Runtime) trySendClassMessage(s *session.Session, message protocol.Message, tick uint64, report *StepReport) error {
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: tick,
		Message:    message,
	}
	report.Metrics.OutboundMessages++
	return s.Connection().TrySend(envelope)
}

func (r *Runtime) retryPendingClassMessages(tick uint64, report *StepReport) {
	if report == nil || len(r.pendingClassMessages) == 0 {
		return
	}
	for _, s := range r.sessions.List() {
		pending := r.pendingClassMessages[s.ID]
		for len(pending) > 0 {
			message := pending[0]
			if err := r.trySendClassMessage(s, message, tick, report); err != nil {
				if errors.Is(err, session.ErrBackpressure) {
					break
				}
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: protocol.DeliveryReliableOrdered, MessageType: message.Type(), Err: err})
				pending = nil
				break
			}
			pending = pending[1:]
		}
		if len(pending) == 0 {
			delete(r.pendingClassMessages, s.ID)
		} else {
			r.pendingClassMessages[s.ID] = pending
		}
	}
	for id := range r.pendingClassMessages {
		if _, ok := r.sessions.Get(id); !ok {
			delete(r.pendingClassMessages, id)
		}
	}
}

func initialClassSelectionRejectionReason(err error) protocol.InitialClassSelectionRejectionReason {
	switch {
	case errors.Is(err, character.ErrInvalidClassAssignment):
		return protocol.InitialClassSelectionInvalidClass
	case errors.Is(err, character.ErrClassAlreadyAssigned):
		return protocol.InitialClassSelectionAlreadyAssigned
	case errors.Is(err, ErrInitialClassAssignmentPending):
		return protocol.InitialClassSelectionAssignmentPending
	case errors.Is(err, ErrInitialClassAssignmentEquipmentIllegal):
		return protocol.InitialClassSelectionEquipmentIllegal
	case errors.Is(err, ErrInitialClassAssignmentRequiresPersistence):
		return protocol.InitialClassSelectionPersistenceUnavailable
	case errors.Is(err, ErrInitialClassAssignmentRequiresTrustedIdentity):
		return protocol.InitialClassSelectionTrustedIdentityRequired
	default:
		return protocol.InitialClassSelectionServerRejected
	}
}
