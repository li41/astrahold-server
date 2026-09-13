package combat

import "github.com/li41/astrahold-server/internal/world"

// ClearTransientStatuses is the entity-incarnation lifecycle boundary for runtime-only combat
// effects. Defeat, formal world leave and managed despawn use this instead of effect-specific
// removal so a future second status cannot survive an old EntityID incarnation.
func (s *Service) ClearTransientStatuses(entityID world.EntityID) {
	if s == nil || entityID == 0 || s.statuses == nil {
		return
	}
	s.statuses.ClearEntity(entityID)
}
