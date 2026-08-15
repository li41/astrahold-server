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

// enqueueCharacterStateSave captures authoritative state while the world owner still
// owns the entity, then hands only immutable data to the process-local save outbox.
// Disk I/O is deliberately performed by the worldd persistence worker, never here.
func (r *Runtime) enqueueCharacterStateSave(sessionID session.ID, entityID world.EntityID, report *StepReport) bool {
	if r.characterStateOutbox == nil {
		return false
	}
	binding, ok := r.characterIdentities.binding(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrCharacterIdentityMissing)
		return false
	}
	// Ephemeral identities are intentionally not persisted. They are local development
	// incarnation keys, not returning-character ownership.
	if binding.Assurance != characteridentity.AssuranceTrusted {
		return false
	}
	state, ok := r.characters.State(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return false
	}
	entity, ok := r.world.Entity(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return false
	}
	snapshot := characterstate.Snapshot{
		World:    r.characterStateWorld,
		HP:       state.HP,
		MaxHP:    state.MaxHP,
		Defeated: state.Defeated,
		Position: entity.Transform.Position,
		Yaw:      entity.Transform.Yaw,
	}
	if state.Defeated {
		// A v2 defeated record is written only when the already-established death-time
		// binding exists. Never invent a context/destination during persistence.
		if r.respawnPolicy == nil {
			recordCharacterStateSaveFailure(report, sessionID, ErrCharacterStateDefeatedRespawnMissing)
			return false
		}
		scheduled, ok := r.respawnPolicy.Pending(entityID)
		if !ok {
			recordCharacterStateSaveFailure(report, sessionID, ErrCharacterStateDefeatedRespawnMissing)
			return false
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
	if _, err := r.characterStateOutbox.Enqueue(binding, snapshot); err != nil {
		recordCharacterStateSaveFailure(report, sessionID, err)
		return false
	}
	if report != nil {
		report.Metrics.CharacterStateSaveIntentsEnqueued++
	}
	return true
}

// markCharacterStateAutosaveBaseline starts the interval at join/register time so a newly
// admitted character is not immediately swept just because the process tick is already high.
func (r *Runtime) markCharacterStateAutosaveBaseline(entityID world.EntityID, tick uint64) {
	if r.config.CharacterStateAutosaveEveryTicks == 0 {
		return
	}
	r.characterStateAutosaveLastTick[entityID] = tick
}

func (r *Runtime) forgetCharacterStateAutosave(entityID world.EntityID) {
	delete(r.characterStateAutosaveLastTick, entityID)
}

// autosaveCharacterStates performs a bounded round-robin sweep of active trusted sessions.
// It captures only immutable authoritative snapshots into the existing process-local outbox;
// journal append/fsync, Store CAS, and checkpoint I/O remain in the worldd worker.
func (r *Runtime) autosaveCharacterStates(tick uint64, report *StepReport) {
	interval := r.config.CharacterStateAutosaveEveryTicks
	if interval == 0 || r.characterStateOutbox == nil {
		return
	}
	budget := r.config.MaxCharacterStateAutosavesPerTick
	if budget <= 0 {
		return
	}
	if report != nil {
		report.Metrics.CharacterStateAutosaveBudget = budget
	}

	sessions := r.sessions.List()
	if len(sessions) == 0 {
		r.characterStateAutosaveCursor = 0
		return
	}
	start := r.characterStateAutosaveCursor % len(sessions)
	if start < 0 {
		start = 0
	}
	attempts := 0
	lastAttemptedIndex := -1
	for checked := 0; checked < len(sessions); checked++ {
		index := (start + checked) % len(sessions)
		s := sessions[index]
		if s.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
			continue
		}
		lastTick, ok := r.characterStateAutosaveLastTick[s.EntityID]
		if !ok || tick < lastTick {
			r.characterStateAutosaveLastTick[s.EntityID] = tick
			continue
		}
		if tick-lastTick < interval {
			continue
		}
		if attempts >= budget {
			if report != nil {
				report.Metrics.CharacterStateAutosaveDeferred++
			}
			r.characterStateAutosaveCursor = index
			return
		}
		attempts++
		lastAttemptedIndex = index
		if report != nil {
			report.Metrics.CharacterStateAutosaveAttempts++
		}
		if r.enqueueCharacterStateSave(s.ID, s.EntityID, report) {
			r.characterStateAutosaveLastTick[s.EntityID] = tick
			if report != nil {
				report.Metrics.CharacterStateAutosaveEnqueued++
			}
		}
	}

	if lastAttemptedIndex >= 0 {
		r.characterStateAutosaveCursor = (lastAttemptedIndex + 1) % len(sessions)
		return
	}
	// No session was due. Rotate one slot anyway so equal-age candidates do not always begin
	// from the same low SessionID when they become eligible together.
	r.characterStateAutosaveCursor = (start + 1) % len(sessions)
}

func recordCharacterStateSaveFailure(report *StepReport, sessionID session.ID, err error) {
	if report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: "enqueue_character_state_save", SessionID: sessionID, Err: err})
	report.Metrics.CharacterStateSaveIntentFailures++
}
