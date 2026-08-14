package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const maxInitialVitalsPerSessionTick = 32

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
	r.dirtyVitalsEntities[entityID] = struct{}{}
}

func (r *Runtime) removeEntityVitals(entityID world.EntityID) {
	delete(r.entityVitalsRevision, entityID)
	delete(r.dirtyVitalsEntities, entityID)
	for _, delivered := range r.sessionVitalsRevision {
		delete(delivered, entityID)
	}
	for _, pending := range r.sessionVitalsPending {
		delete(pending, entityID)
	}
}

func (r *Runtime) removeSessionVitals(id session.ID) {
	delete(r.sessionVitalsRevision, id)
	delete(r.sessionVitalsPending, id)
}

// queueEntityVitalsForSession 只在 Reliable Spawn 成功排入 outbound queue 後呼叫。
// 這讓 initial vitals retry 與 lifecycle known truth 綁在一起，而不是每 tick 掃全世界 Character state。
func (r *Runtime) queueEntityVitalsForSession(sessionID session.ID, entityID world.EntityID) {
	pending := r.sessionVitalsPending[sessionID]
	if pending == nil {
		pending = make(map[world.EntityID]struct{})
		r.sessionVitalsPending[sessionID] = pending
	}
	pending[entityID] = struct{}{}
}

func (r *Runtime) confirmEntityDespawnVitals(sessionID session.ID, entityID world.EntityID) {
	if delivered := r.sessionVitalsRevision[sessionID]; delivered != nil {
		delete(delivered, entityID)
	}
	if pending := r.sessionVitalsPending[sessionID]; pending != nil {
		delete(pending, entityID)
		if len(pending) == 0 {
			delete(r.sessionVitalsPending, sessionID)
		}
	}
}

func (r *Runtime) ensureSessionVitalsDelivered(sessionID session.ID) map[world.EntityID]uint64 {
	delivered := r.sessionVitalsRevision[sessionID]
	if delivered == nil {
		delivered = make(map[world.EntityID]uint64)
		r.sessionVitalsRevision[sessionID] = delivered
	}
	return delivered
}

// replicateEntityVitals 將 Character full-state 視為可重送的 Reliable state，而不是一次性事件。
// Initial full-state 同時受 per-session 32 與 global per-world-tick budget 約束；global budget
// 以 Session round-robin 起點輪轉，避免固定低 ID 在持續 churn 中總是先取得 Vitals quantum。
// Dirty gameplay fan-out 不共用這個 bootstrap budget，仍保留 latest full-state retry semantics。
func (r *Runtime) replicateEntityVitals(tick uint64, report *StepReport) {
	sessions := r.sessions.List()
	report.Metrics.InitialVitalsGlobalBudget = r.config.MaxInitialVitalsPerTick
	globalRemaining := r.config.MaxInitialVitalsPerTick
	startIndex := 0
	if len(sessions) > 0 {
		startIndex = r.vitalsSessionCursor % len(sessions)
		if startIndex < 0 {
			startIndex = 0
		}
	}
	budgetExhaustedNextCursor := -1

	for order := 0; order < len(sessions); order++ {
		index := (startIndex + order) % len(sessions)
		s := sessions[index]
		sessionID := s.ID
		pending := r.sessionVitalsPending[sessionID]
		if len(pending) == 0 {
			continue
		}
		delivered := r.ensureSessionVitalsDelivered(sessionID)
		selected := 0
		for entityID := range pending {
			if !r.replication.Knows(sessionID, entityID) {
				delete(pending, entityID)
				delete(delivered, entityID)
				continue
			}
			state, ok := r.characters.State(entityID)
			if !ok {
				delete(pending, entityID)
				delete(delivered, entityID)
				continue
			}
			revision := r.entityVitalsRevision[entityID]
			if revision == 0 || delivered[entityID] >= revision {
				delete(pending, entityID)
				continue
			}
			if selected >= maxInitialVitalsPerSessionTick || globalRemaining <= 0 {
				break
			}
			if err := r.trySendEntityVitals(s, state.EntityID, state.HP, state.MaxHP, state.Defeated, tick, report); err != nil {
				if errors.Is(err, session.ErrBackpressure) {
					break
				}
				continue
			}
			selected++
			globalRemaining--
			report.Metrics.InitialVitalsGlobalSelected++
			delivered[entityID] = revision
			delete(pending, entityID)
		}
		if len(pending) == 0 {
			delete(r.sessionVitalsPending, sessionID)
		}
		if globalRemaining <= 0 {
			report.Metrics.InitialVitalsGlobalBudgetExhausted = true
			budgetExhaustedNextCursor = (index + 1) % len(sessions)
			break
		}
	}

	if len(sessions) > 0 {
		if budgetExhaustedNextCursor >= 0 {
			r.vitalsSessionCursor = budgetExhaustedNextCursor
		} else {
			r.vitalsSessionCursor = (startIndex + 1) % len(sessions)
		}
	}

	// 若 Session 已離線但其 pending map 還沒被 remove command 清掉，做稀有 cleanup。
	for sessionID := range r.sessionVitalsPending {
		if _, ok := r.sessions.Get(sessionID); !ok {
			delete(r.sessionVitalsPending, sessionID)
			delete(r.sessionVitalsRevision, sessionID)
		}
	}

	if len(r.dirtyVitalsEntities) == 0 {
		return
	}

	// Dirty fan-out 是 O(Sessions × dirty entities)，而不是 O(Sessions × all characters) 每 tick。
	// 若某 Session backpressure，該 entity 保留 dirty，下一 tick retry latest full state。
	for entityID := range r.dirtyVitalsEntities {
		state, ok := r.characters.State(entityID)
		if !ok {
			delete(r.dirtyVitalsEntities, entityID)
			continue
		}
		revision := r.entityVitalsRevision[entityID]
		if revision == 0 {
			delete(r.dirtyVitalsEntities, entityID)
			continue
		}

		allDelivered := true
		for _, s := range sessions {
			if !r.replication.Knows(s.ID, entityID) {
				continue
			}
			delivered := r.ensureSessionVitalsDelivered(s.ID)
			if delivered[entityID] >= revision {
				continue
			}
			if err := r.trySendEntityVitals(s, state.EntityID, state.HP, state.MaxHP, state.Defeated, tick, report); err != nil {
				allDelivered = false
				continue
			}
			delivered[entityID] = revision
			if pending := r.sessionVitalsPending[s.ID]; pending != nil {
				delete(pending, entityID)
				if len(pending) == 0 {
					delete(r.sessionVitalsPending, s.ID)
				}
			}
		}
		if allDelivered {
			delete(r.dirtyVitalsEntities, entityID)
		}
	}
}

func (r *Runtime) trySendEntityVitals(s *session.Session, entityID world.EntityID, hp, maxHP uint32, defeated bool, tick uint64, report *StepReport) error {
	message := protocol.EntityVitalsState{EntityID: entityID, HP: hp, MaxHP: maxHP, Defeated: defeated}
	envelope := protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: s.NextOutboundSequence(protocol.DeliveryReliableOrdered), ServerTick: tick, Message: message}
	report.Metrics.OutboundMessages++
	if err := s.Connection().TrySend(envelope); err != nil {
		if !errors.Is(err, session.ErrBackpressure) {
			report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: envelope.Delivery, MessageType: message.Type(), Err: err})
		}
		return err
	}
	return nil
}
