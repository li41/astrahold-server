package worldruntime

import (
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/equipmentstats"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) equippedInstanceModifiers(entityID world.EntityID) (equipmentstats.Modifiers, error) {
	if r == nil || entityID == 0 { return equipmentstats.Modifiers{}, nil }
	s, ok := r.sessions.GetByEntity(entityID); if !ok || !s.CharacterIdentity.Valid() { return equipmentstats.Modifiers{}, nil }
	inv := r.inventories[s.CharacterIdentity.ID]; if inv == nil { return equipmentstats.Modifiers{}, nil }
	instances := make([]iteminstance.Instance, 0, 7); staticModifiers := make([]equipmentcatalog.StaticModifier, 0, 16)
	appendStatic := func(itemArchetypeID string) { modifiers, found := defaultEquipmentCatalog.StaticModifiersForItem(itemArchetypeID); if found { staticModifiers = append(staticModifiers, modifiers...) } }
	for _, item := range inv.EquippedArchetypeSnapshot() { appendStatic(item.ArchetypeID) }
	for _, equipped := range inv.EquippedInstanceSnapshot() { instances = append(instances, equipped.Item); appendStatic(equipped.Item.ItemArchetypeID) }
	fixed, err := equipmentstats.AggregateStatic(staticModifiers...); if err != nil { return equipmentstats.Modifiers{}, err }
	rolled, err := equipmentstats.Aggregate(instances...); if err != nil { return equipmentstats.Modifiers{}, err }
	return equipmentstats.Merge(fixed, rolled)
}
