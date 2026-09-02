package combat

import "github.com/li41/astrahold-server/internal/world"

// ActionCooldownReadyTick exposes only the authoritative ready tick already committed by Service.
// A zero value means no cooldown has been committed for this actor/action pair.
func (s *Service) ActionCooldownReadyTick(actorEntityID world.EntityID, actionID string) uint64 {
	return s.nextUseTick[cooldownKey{entityID: actorEntityID, actionID: actionID}]
}
