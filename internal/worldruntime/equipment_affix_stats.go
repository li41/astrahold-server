package worldruntime

import (
	"github.com/li41/astrahold-server/internal/equipmentstats"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/world"
)

// equippedInstanceModifiers derives gameplay modifiers only from authoritative unique item
// instances currently equipped by the target player. Legacy low-tier stack equipment has no
// random affixes and therefore contributes nothing here.
func (r *Runtime) equippedInstanceModifiers(entityID world.EntityID) (equipmentstats.Modifiers, error) {
	if r == nil || entityID == 0 {
		return equipmentstats.Modifiers{}, nil
	}
	s, ok := r.sessions.GetByEntity(entityID)
	if !ok || !s.CharacterIdentity.Valid() {
		return equipmentstats.Modifiers{}, nil
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		return equipmentstats.Modifiers{}, nil
	}

	instances := make([]iteminstance.Instance, 0, 2)
	if instance, ok := inv.MainHandInstance(); ok {
		instances = append(instances, instance)
	}
	if instance, ok := inv.OffHandInstance(); ok {
		instances = append(instances, instance)
	}
	return equipmentstats.Aggregate(instances...)
}
