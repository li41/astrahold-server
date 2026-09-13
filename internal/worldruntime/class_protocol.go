package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

// Type117 legacy class-resource feedback and current Type118 target-resource feedback share one
// bounded Reliable queue. The queue limit therefore belongs to generic resource delivery, not to
// the retired fixed-profession model.
const maxPendingResourceMessagesPerSession = 8

// ErrLegacyClassResourceFeedbackBacklog protects only the explicit v27 legacy class-resource
// presentation lane. Fixed-class selection and CharacterClassState publication are retired.
var ErrLegacyClassResourceFeedbackBacklog = errors.New("worldruntime: legacy class resource feedback backlog full")

func protocolClassResourceState(state character.State) (protocol.CharacterClassResourceState, bool) {
	resource := state.ActionResource()
	if state.EntityID == 0 || resource.ID == "" || resource.Max == 0 {
		return protocol.CharacterClassResourceState{}, false
	}
	return protocol.CharacterClassResourceState{
		EntityID:   state.EntityID,
		ResourceID: string(resource.ID),
		Current:    resource.Current,
		Max:        resource.Max,
	}, true
}

// queueCurrentActionResourceState is the no-StepReport publication path used while establishing
// or transferring a trusted session. The queued message is still encoded as Protocol v27 Type117,
// but the source state is the Server-owned classless action resource.
func (r *Runtime) queueCurrentActionResourceState(s *session.Session) {
	if s == nil {
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
	pending := r.pendingResourceMessages[s.ID]
	if len(pending) >= maxPendingResourceMessagesPerSession {
		_ = s.Connection().Close()
		return
	}
	r.pendingResourceMessages[s.ID] = append(pending, message)
}

// sendCurrentActionResourceState is the generic gameplay-facing publication path. Protocol v27
// still encodes this state as legacy CharacterClassResourceState; callers must not infer a class.
func (r *Runtime) sendCurrentActionResourceState(s *session.Session, report *StepReport) {
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
	r.sendLegacyClassResourceMessage(s, message, report)
}

// sendLegacyClassResourceMessage is compatibility-only delivery for CharacterClassResourceState.
// Current gameplay must not use this lane for class identity or selection results.
func (r *Runtime) sendLegacyClassResourceMessage(s *session.Session, message protocol.Message, report *StepReport) {
	if s == nil || message == nil || report == nil {
		return
	}
	pending := r.pendingResourceMessages[s.ID]
	if len(pending) > 0 {
		if len(pending) >= maxPendingResourceMessagesPerSession {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "legacy_class_resource_feedback", SessionID: s.ID, Err: ErrLegacyClassResourceFeedbackBacklog})
			_ = s.Connection().Close()
			return
		}
		r.pendingResourceMessages[s.ID] = append(pending, message)
		return
	}
	if err := r.trySendResourceMessage(s, message, report.Tick, report); err != nil {
		if errors.Is(err, session.ErrBackpressure) {
			r.pendingResourceMessages[s.ID] = append(r.pendingResourceMessages[s.ID], message)
			return
		}
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: protocol.DeliveryReliableOrdered, MessageType: message.Type(), Err: err})
	}
}

// trySendResourceMessage is the common Reliable send primitive for resource presentation state.
// It carries both legacy Type117 class-resource compatibility and current Type118 target resources;
// gameplay ownership remains in the authoritative character/action/target-resource services.
func (r *Runtime) trySendResourceMessage(s *session.Session, message protocol.Message, tick uint64, report *StepReport) error {
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: tick,
		Message:    message,
	}
	report.Metrics.OutboundMessages++
	return s.Connection().TrySend(envelope)
}

func (r *Runtime) retryPendingResourceMessages(tick uint64, report *StepReport) {
	if report == nil || len(r.pendingResourceMessages) == 0 {
		return
	}
	for _, s := range r.sessions.List() {
		pending := r.pendingResourceMessages[s.ID]
		for len(pending) > 0 {
			message := pending[0]
			if err := r.trySendResourceMessage(s, message, tick, report); err != nil {
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
			delete(r.pendingResourceMessages, s.ID)
		} else {
			r.pendingResourceMessages[s.ID] = pending
		}
	}
	for id := range r.pendingResourceMessages {
		if _, ok := r.sessions.Get(id); !ok {
			delete(r.pendingResourceMessages, id)
		}
	}
}
