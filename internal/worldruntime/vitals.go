package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) broadcastEntityVitals(entityID world.EntityID, tick uint64, report *StepReport) {
	for _, s := range r.sessions.List() {
		if r.replication.Knows(s.ID, entityID) {
			r.sendEntityVitals(s, entityID, tick, report)
		}
	}
}

func (r *Runtime) sendEntityVitals(s *session.Session, entityID world.EntityID, tick uint64, report *StepReport) {
	state, ok := r.characters.State(entityID)
	if !ok { return }
	message := protocol.EntityVitalsState{EntityID:state.EntityID,HP:state.HP,MaxHP:state.MaxHP,Defeated:state.Defeated}
	envelope := protocol.Envelope{Delivery:protocol.DeliveryReliableOrdered,Sequence:s.NextOutboundSequence(protocol.DeliveryReliableOrdered),ServerTick:tick,Message:message}
	report.Metrics.OutboundMessages++
	if err := s.Connection().TrySend(envelope); err != nil {
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID,Delivery:envelope.Delivery,MessageType:message.Type(),Err:err})
	}
}
