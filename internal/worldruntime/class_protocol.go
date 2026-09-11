package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const maxPendingClassMessagesPerSession = 8

// ErrLegacyClassResourceFeedbackBacklog protects only the explicit v27 legacy class-resource
// presentation lane. Fixed-class selection and CharacterClassState publication are retired.
var ErrLegacyClassResourceFeedbackBacklog = errors.New("worldruntime: legacy class resource feedback backlog full")

// EnqueueInitialClassSelection is a v27 source-compatibility tombstone. The production gateway no
// longer routes this wire message and direct callers cannot mutate profession truth through it.
func (r *Runtime) EnqueueInitialClassSelection(_ session.ID, _ uint32, _ protocol.ClientInitialClassSelection) error {
	return ErrFixedClassSelectionRetired
}

// EnqueueFencedInitialClassSelection is a v27 source-compatibility tombstone. It never queues work.
func (r *Runtime) EnqueueFencedInitialClassSelection(_ SessionOwnershipFence, _ uint32, _ protocol.ClientInitialClassSelection) error {
	return ErrFixedClassSelectionRetired
}

func protocolClassResourceState(state character.State) (protocol.CharacterClassResourceState, bool) {
	if state.EntityID == 0 || state.ClassResourceID == "" || state.MaxClassResource == 0 {
		return protocol.CharacterClassResourceState{}, false
	}
	return protocol.CharacterClassResourceState{
		EntityID:   state.EntityID,
		ResourceID: string(state.ClassResourceID),
		Current:    state.ClassResource,
		Max:        state.MaxClassResource,
	}, true
}

// queueCurrentClassState intentionally no longer publishes CharacterClassState. Current classless
// characters have no profession identity. A legacy v6-v8 E2E fixture may still expose its runtime
// class-resource meter while Protocol v27 compatibility testing remains necessary.
func (r *Runtime) queueCurrentClassState(s *session.Session) {
	if s == nil {
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		return
	}
	resourceState, ok := protocolClassResourceState(state)
	if !ok {
		return
	}
	pending := r.pendingClassMessages[s.ID]
	if len(pending)+1 > maxPendingClassMessagesPerSession {
		_ = s.Connection().Close()
		return
	}
	r.pendingClassMessages[s.ID] = append(pending, resourceState)
}

func (r *Runtime) sendCurrentClassResourceState(s *session.Session, report *StepReport) {
	if s == nil || report == nil {
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		return
	}
	message, ok := protocolClassResourceState(state)
	if !ok {
		return
	}
	r.sendClassMessage(s, message, report)
}

// sendClassMessage is compatibility-only delivery for CharacterClassResourceState. Current gameplay
// must not use this lane for class identity or selection results.
func (r *Runtime) sendClassMessage(s *session.Session, message protocol.Message, report *StepReport) {
	if s == nil || message == nil || report == nil {
		return
	}
	pending := r.pendingClassMessages[s.ID]
	if len(pending) > 0 {
		if len(pending) >= maxPendingClassMessagesPerSession {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "legacy_class_resource_feedback", SessionID: s.ID, Err: ErrLegacyClassResourceFeedbackBacklog})
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
