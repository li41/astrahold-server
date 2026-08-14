package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) ensureEntityVitalsRevision(entityID world.EntityID) {
	if r.entityVitalsRevision[entityID] == 0 {
		r.entityVitalsRevision[entityID] = 1
	}
}

func (r *Runtime) markEntityVitalsDirty(entityID world.EntityID) {
	r.entityVitalsRevision[entityID]++
	if r.entityVitalsRevision[entityID] == 0 {
		r.entityVitalsRevision[entityID] = 1
	}
}

func (r *Runtime) removeEntityVitals(entityID world.EntityID) {
	delete(r.entityVitalsRevision, entityID)
	for _, delivered := range r.sessionVitalsRevision {
		delete(delivered, entityID)
	}
}

func (r *Runtime) removeSessionVitals(id session.ID) {
	delete(r.sessionVitalsRevision, id)
}

// replicateEntityVitals 將 Character full-state 視為可重送的 Reliable state，而不是一次性事件。
// ErrBackpressure 只代表本 tick 延後，只有成功寫入 outbound queue 才更新 session revision。
func (r *Runtime) replicateEntityVitals(tick uint64, report *StepReport) {
	states := r.characters.States()
	if len(states) == 0 {
		return
	}

	activeSessions := make(map[session.ID]struct{})
	for _, s := range r.sessions.List() {
		activeSessions[s.ID] = struct{}{}
		delivered := r.sessionVitalsRevision[s.ID]
		if delivered == nil {
			delivered = make(map[world.EntityID]uint64)
			r.sessionVitalsRevision[s.ID] = delivered
		}

		for entityID := range delivered {
			if !r.replication.Knows(s.ID, entityID) {
				delete(delivered, entityID)
			}
		}

		for _, state := range states {
			if !r.replication.Knows(s.ID, state.EntityID) {
				continue
			}
			revision := r.entityVitalsRevision[state.EntityID]
			if revision == 0 || delivered[state.EntityID] >= revision {
				continue
			}
			message := protocol.EntityVitalsState{EntityID:state.EntityID,HP:state.HP,MaxHP:state.MaxHP,Defeated:state.Defeated}
			envelope := protocol.Envelope{Delivery:protocol.DeliveryReliableOrdered,Sequence:s.NextOutboundSequence(protocol.DeliveryReliableOrdered),ServerTick:tick,Message:message}
			report.Metrics.OutboundMessages++
			if err := s.Connection().TrySend(envelope); err != nil {
				if errors.Is(err, session.ErrBackpressure) {
					continue
				}
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID,Delivery:envelope.Delivery,MessageType:message.Type(),Err:err})
				continue
			}
			delivered[state.EntityID] = revision
		}
	}

	for id := range r.sessionVitalsRevision {
		if _, ok := activeSessions[id]; !ok {
			delete(r.sessionVitalsRevision, id)
		}
	}
}
