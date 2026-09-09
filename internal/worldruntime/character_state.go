package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrCharacterStateDefeatedRespawnMissing = errors.New("worldruntime: defeated character durable respawn truth missing")

func WithCharacterStateOutbox(outbox *characterstate.Outbox, worldRef characterstate.WorldRef) Option {
	return func(r *Runtime) {
		r.characterStateOutbox = outbox
		r.characterStateWorld = worldRef
	}
}

// captureCharacterStateSnapshot reads only world-owner gameplay truth and produces immutable
// persistence input. A completion transaction may project a target ClassID into saves while its
// live gameplay mutation is waiting for durability acknowledgement. This prevents autosave/leave
// from erasing an assignment that is already in the durable pipeline without making pending state
// authoritative for combat/equipment/gameplay.
func (r *Runtime) captureCharacterStateSnapshot(sessionID session.ID, entityID world.EntityID, report *StepReport) (characteridentity.Binding, characterstate.Snapshot, bool) {
	binding, ok := r.characterIdentities.binding(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrCharacterIdentityMissing)
		return characteridentity.Binding{}, characterstate.Snapshot{}, false
	}
	if binding.Assurance != characteridentity.AssuranceTrusted {
		return characteridentity.Binding{}, characterstate.Snapshot{}, false
	}
	state, ok := r.characters.State(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return characteridentity.Binding{}, characterstate.Snapshot{}, false
	}
	entity, ok := r.world.Entity(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return characteridentity.Binding{}, characterstate.Snapshot{}, false
	}
	inventoryState, err := durableInventoryState(r.inventories[binding.ID])
	if err != nil {
		recordCharacterStateSaveFailure(report, sessionID, err)
		return characteridentity.Binding{}, characterstate.Snapshot{}, false
	}

	classForSave := state.ClassID
	if r.characterStateOutbox != nil {
		if pending, exists := r.characterStateOutbox.CompletionForCharacter(binding.ID); exists && pending.Snapshot.ClassID != "" {
			classForSave = pending.Snapshot.ClassID
		}
	}
	snapshot := characterstate.Snapshot{
		World:     r.characterStateWorld,
		ClassID:   classForSave,
		HP:        state.HP,
		MaxHP:     state.MaxHP,
		MP:        state.MP,
		MaxMP:     state.MaxMP,
		Defeated:  state.Defeated,
		Position:  entity.Transform.Position,
		Yaw:       entity.Transform.Yaw,
		Inventory: inventoryState,
	}
	if state.Defeated {
		// Defeated records are written only when the already-established death-time binding exists.
		// Never invent a context/destination during persistence.
		if r.respawnPolicy == nil {
			recordCharacterStateSaveFailure(report, sessionID, ErrCharacterStateDefeatedRespawnMissing)
			return characteridentity.Binding{}, characterstate.Snapshot{}, false
		}
		scheduled, ok := r.respawnPolicy.Pending(entityID)
		if !ok {
			recordCharacterStateSaveFailure(report, sessionID, ErrCharacterStateDefeatedRespawnMissing)
			return characteridentity.Binding{}, characterstate.Snapshot{}, false
		}
		remaining := uint64(0)
		if report != nil && scheduled.DueTick > report.Tick {
			remaining = scheduled.DueTick - report.Tick
		}
		checkpointID, _ := r.respawnPolicy.Checkpoint(entityID)
		snapshot.Respawn = characterstate.DefeatedRespawn{
			Context:        scheduled.Context,
			SpawnPointID:   scheduled.SpawnPointID,
			SpawnClass:     scheduled.SpawnClass,
			Position:       scheduled.Position,
			RemainingTicks: remaining,
			CheckpointID:   checkpointID,
		}
	}
	return binding, snapshot, true
}

// enqueueCharacterStateSave captures authoritative state while the world owner still
// owns the entity, then hands only immutable data to the process-local save outbox.
// Disk I/O is deliberately performed by the worldd persistence worker, never here.
func (r *Runtime) enqueueCharacterStateSave(sessionID session.ID, entityID world.EntityID, report *StepReport) bool {
	if r.characterStateOutbox == nil {
		return false
	}
	binding, snapshot, ok := r.captureCharacterStateSnapshot(sessionID, entityID, report)
	if !ok {
		return false
	}
	if _, err := r.characterStateOutbox.Enqueue(binding, snapshot); err != nil {
		recordCharacterStateSaveFailure(report, sessionID, err)
		return false
	}
	if report != nil {
		report.Metrics.CharacterStateSaveIntentsEnqueued++
	}
	return true
}

func characterStateAutosaveDueTick(base, interval uint64) uint64 {
	if interval > ^uint64(0)-base { return ^uint64(0) }
	return base + interval
}
func characterStateAutosaveRetryTick(tick uint64) uint64 {
	if tick == ^uint64(0) { return tick }
	return tick + 1
}
func earlierAutosaveTick(current, candidate uint64) uint64 {
	if candidate == 0 { return current }
	if current == 0 || candidate < current { return candidate }
	return current
}

func (r *Runtime) markCharacterStateAutosaveBaseline(entityID world.EntityID, tick uint64) {
	interval := r.config.CharacterStateAutosaveEveryTicks
	if interval == 0 { return }
	binding, ok := r.characterIdentities.binding(entityID)
	if !ok || binding.Assurance != characteridentity.AssuranceTrusted { return }
	r.characterStateAutosaveLastTick[entityID] = tick
	r.characterStateAutosaveNextTick = earlierAutosaveTick(r.characterStateAutosaveNextTick, characterStateAutosaveDueTick(tick, interval))
}
func (r *Runtime) forgetCharacterStateAutosave(entityID world.EntityID) { delete(r.characterStateAutosaveLastTick, entityID) }

func (r *Runtime) autosaveCharacterStates(tick uint64, report *StepReport) {
	interval := r.config.CharacterStateAutosaveEveryTicks
	if interval == 0 || r.characterStateOutbox == nil { return }
	budget := r.config.MaxCharacterStateAutosavesPerTick
	if budget <= 0 { return }
	if r.characterStateAutosaveNextTick != 0 && tick < r.characterStateAutosaveNextTick { return }
	if report != nil { report.Metrics.CharacterStateAutosaveBudget = budget }

	sessions := r.sessions.List()
	if len(sessions) == 0 {
		r.characterStateAutosaveCursor = 0
		r.characterStateAutosaveNextTick = 0
		return
	}
	start := r.characterStateAutosaveCursor % len(sessions)
	if start < 0 { start = 0 }
	attempts := 0
	lastAttemptedIndex := -1
	nextSweep := uint64(0)
	trustedSeen := false
	for checked := 0; checked < len(sessions); checked++ {
		index := (start + checked) % len(sessions)
		s := sessions[index]
		if s.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted { continue }
		trustedSeen = true
		lastTick, ok := r.characterStateAutosaveLastTick[s.EntityID]
		if !ok || tick < lastTick {
			lastTick = tick
			r.characterStateAutosaveLastTick[s.EntityID] = tick
		}
		dueTick := characterStateAutosaveDueTick(lastTick, interval)
		if tick < dueTick {
			nextSweep = earlierAutosaveTick(nextSweep, dueTick)
			continue
		}
		if attempts >= budget {
			if report != nil { report.Metrics.CharacterStateAutosaveBudgetExhausted = true }
			r.characterStateAutosaveCursor = index
			r.characterStateAutosaveNextTick = characterStateAutosaveRetryTick(tick)
			return
		}
		attempts++
		lastAttemptedIndex = index
		if report != nil { report.Metrics.CharacterStateAutosaveAttempts++ }
		if r.enqueueCharacterStateSave(s.ID, s.EntityID, report) {
			r.characterStateAutosaveLastTick[s.EntityID] = tick
			nextSweep = earlierAutosaveTick(nextSweep, characterStateAutosaveDueTick(tick, interval))
			if report != nil { report.Metrics.CharacterStateAutosaveEnqueued++ }
		} else {
			nextSweep = earlierAutosaveTick(nextSweep, characterStateAutosaveRetryTick(tick))
		}
	}
	if !trustedSeen { r.characterStateAutosaveNextTick = 0 } else { r.characterStateAutosaveNextTick = nextSweep }
	if lastAttemptedIndex >= 0 {
		r.characterStateAutosaveCursor = (lastAttemptedIndex + 1) % len(sessions)
		return
	}
	r.characterStateAutosaveCursor = (start + 1) % len(sessions)
}

func recordCharacterStateSaveFailure(report *StepReport, sessionID session.ID, err error) {
	if report == nil { return }
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: "enqueue_character_state_save", SessionID: sessionID, Err: err})
	report.Metrics.CharacterStateSaveIntentFailures++
}
