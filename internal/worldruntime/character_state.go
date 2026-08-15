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
func (r *Runtime) enqueueCharacterStateSave(sessionID session.ID, entityID world.EntityID, report *StepReport) {
	if r.characterStateOutbox == nil {
		return
	}
	binding, ok := r.characterIdentities.binding(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrCharacterIdentityMissing)
		return
	}
	// Ephemeral identities are intentionally not persisted. They are local development
	// incarnation keys, not returning-character ownership.
	if binding.Assurance != characteridentity.AssuranceTrusted {
		return
	}
	state, ok := r.characters.State(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return
	}
	entity, ok := r.world.Entity(entityID)
	if !ok {
		recordCharacterStateSaveFailure(report, sessionID, ErrSessionEntityNotFound)
		return
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
			return
		}
		scheduled, ok := r.respawnPolicy.Pending(entityID)
		if !ok {
			recordCharacterStateSaveFailure(report, sessionID, ErrCharacterStateDefeatedRespawnMissing)
			return
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
		return
	}
	if report != nil {
		report.Metrics.CharacterStateSaveIntentsEnqueued++
	}
}

func recordCharacterStateSaveFailure(report *StepReport, sessionID session.ID, err error) {
	if report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: "enqueue_character_state_save", SessionID: sessionID, Err: err})
	report.Metrics.CharacterStateSaveIntentFailures++
}
