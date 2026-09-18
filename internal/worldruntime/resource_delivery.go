package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

// Type118 target-resource feedback uses one bounded Reliable queue per owning session.
const maxPendingResourceMessagesPerSession = 8

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
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
					SessionID:   s.ID,
					Delivery:    protocol.DeliveryReliableOrdered,
					MessageType: message.Type(),
					Err:         err,
				})
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
