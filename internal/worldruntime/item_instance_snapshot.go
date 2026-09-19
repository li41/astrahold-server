package worldruntime

import (
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
)

func protocolItemInstanceState(instance iteminstance.Instance) protocol.ItemInstanceState {
	affixes := make([]protocol.ItemAffixState, 0, len(instance.Affixes))
	for _, affix := range instance.Affixes { affixes = append(affixes, protocol.ItemAffixState{AffixID:string(affix.ID), Strength:affix.Strength, Value:affix.Value}) }
	return protocol.ItemInstanceState{ItemInstanceID:string(instance.ID), ItemArchetypeID:instance.ItemArchetypeID, EnhancementLevel:instance.EnhancementLevel, Affixes:affixes}
}
func buildInventoryInstanceSnapshot(inv *inventory.Inventory) (protocol.InventoryInstanceSnapshot, error) {
	snapshot := protocol.InventoryInstanceSnapshot{Revision:inv.Revision()}; instances := inv.InstanceSnapshot(); if len(instances) == 0 { return snapshot, nil }
	snapshot.Items = make([]protocol.ItemInstanceState, 0, len(instances)); for _, instance := range instances { if err := validateDurableEquipmentInstance(instance, "", ""); err != nil { return protocol.InventoryInstanceSnapshot{}, err }; snapshot.Items = append(snapshot.Items, protocolItemInstanceState(instance)) }; return snapshot, nil
}
func buildEquipmentInstanceSnapshot(inv *inventory.Inventory) (protocol.EquipmentInstanceSnapshot, error) {
	snapshot := protocol.EquipmentInstanceSnapshot{Revision:inv.EquipmentRevision()}
	for _, equipped := range inv.EquippedInstanceSnapshot() {
		kind, ok := expectedEquipmentKind(equipped.Slot); if !ok { return protocol.EquipmentInstanceSnapshot{}, ErrEquipmentItemNotAllowed }; catalogSlot, ok := catalogEquipmentSlot(equipped.Slot); if !ok { return protocol.EquipmentInstanceSnapshot{}, ErrEquipmentItemNotAllowed }
		if err := validateDurableEquipmentInstance(equipped.Item, kind, catalogSlot); err != nil { return protocol.EquipmentInstanceSnapshot{}, err }; slot, ok := protocolEquipmentSlot(equipped.Slot); if !ok { return protocol.EquipmentInstanceSnapshot{}, ErrEquipmentItemNotAllowed }
		snapshot.Slots = append(snapshot.Slots, protocol.EquipmentInstanceSlotState{Slot:slot, Item:protocolItemInstanceState(equipped.Item)})
	}
	return snapshot, nil
}
