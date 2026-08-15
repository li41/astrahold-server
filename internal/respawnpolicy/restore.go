package respawnpolicy

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidRestore = errors.New("respawnpolicy: invalid durable restore")

// ValidateScheduledRestore verifies that an immutable death-time binding still
// exactly matches the currently loaded policy definition. It never re-selects a
// destination, so checkpoint/config changes cannot silently rewrite an old death.
func (s *Service) ValidateScheduledRestore(scheduled Scheduled) error {
	if scheduled.EntityID == 0 {
		return fmt.Errorf("%w: entity", ErrInvalidRestore)
	}
	rule, ok := s.rules[scheduled.Context]
	if !ok {
		return fmt.Errorf("%w: context=%q", ErrInvalidRestore, scheduled.Context)
	}
	point, ok := s.points[scheduled.SpawnPointID]
	if !ok {
		return fmt.Errorf("%w: spawn_point=%q", ErrInvalidRestore, scheduled.SpawnPointID)
	}
	if _, allowed := rule.allowed[point.Class]; !allowed {
		return fmt.Errorf("%w: context=%q disallows spawn_class=%q", ErrInvalidRestore, scheduled.Context, point.Class)
	}
	if point.Class != scheduled.SpawnClass || point.Position() != scheduled.Position {
		return fmt.Errorf("%w: spawn binding drift point=%q", ErrInvalidRestore, scheduled.SpawnPointID)
	}
	return nil
}

// RestoreScheduled installs one already-validated death-time binding for a new
// entity incarnation. DueTick is supplied in the current process tick domain.
func (s *Service) RestoreScheduled(scheduled Scheduled) error {
	if err := s.ValidateScheduledRestore(scheduled); err != nil {
		return err
	}
	s.pending[scheduled.EntityID] = scheduled
	return nil
}

// ValidateCheckpointRestore verifies durable checkpoint truth without running the
// live proximity-acquisition rule. The caller is trusted restore code, not gameplay input.
func (s *Service) ValidateCheckpointRestore(entityID world.EntityID, spawnPointID string) error {
	if entityID == 0 {
		return fmt.Errorf("%w: entity", ErrInvalidRestore)
	}
	if spawnPointID == "" {
		return nil
	}
	point, ok := s.points[spawnPointID]
	if !ok || point.Class != SpawnClassCheckpoint || point.CheckpointActivationRadius <= 0 {
		return fmt.Errorf("%w: checkpoint=%q", ErrInvalidRestore, spawnPointID)
	}
	return nil
}

// RestoreCheckpoint installs the settled post-penalty checkpoint state. Empty means
// no acquired checkpoint (including the current PvE checkpoint-forfeiture result).
func (s *Service) RestoreCheckpoint(entityID world.EntityID, spawnPointID string) error {
	if err := s.ValidateCheckpointRestore(entityID, spawnPointID); err != nil {
		return err
	}
	if spawnPointID == "" {
		delete(s.checkpoints, entityID)
		return nil
	}
	s.checkpoints[entityID] = spawnPointID
	return nil
}
