package worldruntime

import (
	"github.com/li41/astrahold-server/internal/playerarchetype"
	"github.com/li41/astrahold-server/internal/world"
)

// canonicalizeJoinEntity applies Server-owned defaults before an entity enters authoritative
// world state. A non-empty archetype is already an explicit stable identity and is preserved.
func canonicalizeJoinEntity(entity world.EntityState) world.EntityState {
	if entity.Kind == world.EntityPlayer && entity.ArchetypeID == "" {
		entity.ArchetypeID = playerarchetype.DefaultID
	}
	return entity
}
