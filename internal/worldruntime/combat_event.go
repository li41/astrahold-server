package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

// emitCombatEvent delivers event truth only to direct participants in this bounded foundation.
// Authoritative HP/world state still converges through EntityVitalsState/WorldDynamicState, so
// outbound backpressure can lose presentation feedback without rolling back gameplay state.
// AOI observer fan-out is intentionally deferred until encounter profiling justifies its cost.
func (r *Runtime) emitCombatEvent(event protocol.CombatEvent, tick uint64, report *StepReport) {
	if event.ActionInstanceID == 0 || event.ActorEntityID == 0 || event.ActionID == "" {
		return
	}
	for _, s := range r.sessions.List() {
		if s.EntityID != event.ActorEntityID && (event.TargetEntityID == 0 || s.EntityID != event.TargetEntityID) {
			continue
		}
		envelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    event,
		}
		if err := s.Connection().TrySend(envelope); err != nil {
			report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
				SessionID:   s.ID,
				Delivery:    protocol.DeliveryReliableOrdered,
				MessageType: protocol.MessageCombatEvent,
				Err:         err,
			})
			if err == session.ErrConnectionClosed {
				continue
			}
		}
	}
}
