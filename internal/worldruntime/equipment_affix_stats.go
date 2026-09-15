package worldruntime

import (
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/equipmentstats"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/world"
)

// equippedInstanceModifiers retains its historical name because existing combat paths already use
// it as their equipment-stat seam. It now composes immutable ItemArchetype static modifiers with
// random affixes from authoritative unique instances. Low-tier archetype equipment can therefore
// contribute fixed power without becoming a fake ItemInstance, while mid/high instances keep their
// persisted rolled quality on top of the same base archetype truth.
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
	staticModifiers := make([]equipmentcatalog.StaticModifier, 0, 4)
	appendStatic := func(itemArchetypeID string) {
		modifiers, found := defaultEquipmentCatalog.StaticModifiersForItem(itemArchetypeID)
		if found {
			staticModifiers = append(staticModifiers, modifiers...)
		}
	}

	if instance, ok := inv.MainHandInstance(); ok {
		instances = append(instances, instance)
		appendStatic(instance.ItemArchetypeID)
	} else if itemArchetypeID := inv.MainHand(); itemArchetypeID != "" {
		appendStatic(itemArchetypeID)
	}
	if instance, ok := inv.OffHandInstance(); ok {
		instances = append(instances, instance)
		appendStatic(instance.ItemArchetypeID)
	} else if itemArchetypeID := inv.OffHand(); itemArchetypeID != "" {
		appendStatic(itemArchetypeID)
	}

	fixed, err := equipmentstats.AggregateStatic(staticModifiers...)
	if err != nil {
		return equipmentstats.Modifiers{}, err
	}
	rolled, err := equipmentstats.Aggregate(instances...)
	if err != nil {
		return equipmentstats.Modifiers{}, err
	}
	return equipmentstats.Merge(fixed, rolled)
}
