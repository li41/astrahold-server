package worldruntime

import (
	"fmt"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

type preparedDefeatedRestore struct {
	scheduled    respawnpolicy.Scheduled
	checkpointID string
}

// prepareDefeatedRestore converts durable remaining ticks into the current process
// tick domain and validates policy provenance without mutating runtime state.
func (r *Runtime) prepareDefeatedRestore(entityID world.EntityID, tick uint64, durable characterstate.DefeatedRespawn) (preparedDefeatedRestore, error) {
	if r.respawnPolicy == nil {
		return preparedDefeatedRestore{}, ErrCharacterRestoreRespawnPolicyUnavailable
	}
	if durable.RemainingTicks > ^uint64(0)-tick {
		return preparedDefeatedRestore{}, fmt.Errorf("%w: respawn due tick overflow", ErrCharacterRestoreInvalid)
	}
	scheduled := respawnpolicy.Scheduled{
		EntityID:     entityID,
		Context:      durable.Context,
		SpawnPointID: durable.SpawnPointID,
		SpawnClass:   durable.SpawnClass,
		Position:     durable.Position,
		DueTick:      tick + durable.RemainingTicks,
	}
	if err := r.respawnPolicy.ValidateScheduledRestore(scheduled); err != nil {
		return preparedDefeatedRestore{}, fmt.Errorf("%w: %v", ErrCharacterRestoreInvalid, err)
	}
	if err := r.respawnPolicy.ValidateCheckpointRestore(entityID, durable.CheckpointID); err != nil {
		return preparedDefeatedRestore{}, fmt.Errorf("%w: %v", ErrCharacterRestoreInvalid, err)
	}
	return preparedDefeatedRestore{scheduled: scheduled, checkpointID: durable.CheckpointID}, nil
}

func (r *Runtime) installDefeatedRestore(prepared preparedDefeatedRestore) error {
	if err := r.respawnPolicy.RestoreCheckpoint(prepared.scheduled.EntityID, prepared.checkpointID); err != nil {
		return err
	}
	if err := r.respawnPolicy.RestoreScheduled(prepared.scheduled); err != nil {
		r.respawnPolicy.Remove(prepared.scheduled.EntityID)
		return err
	}
	return nil
}
