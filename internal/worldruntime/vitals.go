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

// staggeredWorkBudget 把原本每個 world tick 的 work budget 搬離 snapshot tick，同時保留
// 一個完整 snapshot cycle 的理論 capacity。snapshotEvery=2 時，base 2500 -> [0,5000]。
func staggeredWorkBudget(base int, tick, snapshotEvery uint64) int {
	if base <= 0 {
		return 0
	}
	if snapshotEvery <= 1 {
		return base
	}
	if tick%snapshotEvery == 0 {
		return 0
	}
	numerator := base * int(snapshotEvery)
	denominator := int(snapshotEvery - 1)
	return (numerator + denominator - 1) / denominator
}

// Initial Vitals 與 lifecycle 使用 phase-sensitive budgeting，並與 snapshot tick 錯峰。
// Global 與 per-session budget 都套同一比例，否則雖然 global cycle capacity 不變，單一
// Session 仍會從每50ms最多32筆退化成每100ms最多32筆，拖長 semantic convergence。
// Dirty gameplay Vitals 不受這個 initial-state budget 限制。
func (r *Runtime) replicateEntityVitals(tick uint64, report *StepReport) {
	sessions := r.sessions.List()
	baseGlobalBudget := r.config.MaxInitialVitalsPerTick
	if r.lifecycleChurnActive {
		baseGlobalBudget = r.config.MaxChurnInitialVitalsPerTick
	}
	globalBudget := staggeredWorkBudget(baseGlobalBudget, tick, r.config.SnapshotEveryTicks)
	perSessionBudget := staggeredWorkBudget(maxInitialVitalsPerSessionTick, tick, r.config.SnapshotEveryTicks)
	report.Metrics.InitialVitalsGlobalBudget = globalBudget
	globalRemaining := globalBudget
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
			if selected >= perSessionBudget || globalRemaining <= 0 {
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

	for sessionID := range r.sessionVitalsPending {
		if _, ok := r.sessions.Get(sessionID); !ok {
			delete(r.sessionVitalsPending, sessionID)
			delete(r.sessionVitalsRevision, sessionID)
		}
	}

	// 只在 snapshot tick 判斷 churn phase 是否完成，避免兩個 snapshot 之間 lifecycle metrics 為 0 時誤清。
	if r.lifecycleChurnActive && tick%r.config.SnapshotEveryTicks == 0 &&
		report.Metrics.LifecycleGlobalSelected == 0 && report.Metrics.SpawnDeferred == 0 && report.Metrics.DespawnDeferred == 0 &&
		len(r.sessionVitalsPending) == 0 {
		r.lifecycleChurnActive = false
	}

	if len(r.dirtyVitalsEntities) == 0 {
		return
	}

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
