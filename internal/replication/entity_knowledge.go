package replication

import "github.com/li41/astrahold-server/internal/world"

// KnownByAny reports whether any registered replication view still treats entityID as a
// successfully queued Reliable EntitySpawn. It is intentionally a rare lifecycle query, not a
// steady-state replication hot-path primitive.
//
// A server-owned entity that reuses the same EntityID for a later incarnation must not re-enter
// the world until this returns false. Otherwise a slow observer that has not yet accepted the
// prior EntityDespawn could mistake the new incarnation for the old one and never receive a
// fresh EntitySpawn.
func (s *Service) KnownByAny(entityID world.EntityID) bool {
	for _, state := range s.views {
		if state == nil {
			continue
		}
		if _, known := state.known[entityID]; known {
			return true
		}
	}
	return false
}
