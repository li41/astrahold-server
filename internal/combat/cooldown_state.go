package combat

import "github.com/li41/astrahold-server/internal/world"

// ActionCooldownReadyTick exposes only Server-owned cooldown state for authoritative UX feedback.
// A zero value means the action has no committed cooldown for this actor.
func (s *Service) ActionCooldownReadyTick(actorEntityID world.EntityID, actionID string) uint64 {
	if s == nil || actorEntityID == 0 || actionID == "" {
		return 0
	}
	return s.nextUseTick[cooldownKey{entityID: actorEntityID, actionID: actionID}]
}
