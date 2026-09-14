package worldruntime

import (
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
)

func protocolItemInstanceState(instance iteminstance.Instance) protocol.ItemInstanceState {
	affixes := make([]protocol.ItemAffixState, 0, len(instance.Affixes))
	for _, affix := range instance.Affixes {
		affixes = append(affixes, protocol.ItemAffixState{
			AffixID:  string(affix.ID),
			Strength: affix.Strength,
			Value:    affix.Value,
		})
	}
	return protocol.ItemInstanceState{
		ItemInstanceID:  string(instance.ID),
		ItemArchetypeID: instance.ItemArchetypeID,
		Affixes:         affixes,
	}
}

// buildInventoryInstanceSnapshot builds the Protocol v28 complete unique-instance inventory view.
// An empty Items set is meaningful and must still be published so Clients can clear stale state.
func buildInventoryInstanceSnapshot(inv *inventory.Inventory) (protocol.InventoryInstanceSnapshot, error) {
	snapshot := protocol.InventoryInstanceSnapshot{Revision: inv.Revision()}
	instances := inv.InstanceSnapshot()
	if len(instances) == 0 {
		return snapshot, nil
	}
	snapshot.Items = make([]protocol.ItemInstanceState, 0, len(instances))
	for _, instance := range instances {
		if err := validateDurableEquipmentInstance(instance, "", ""); err != nil {
			return protocol.InventoryInstanceSnapshot{}, err
		}
		snapshot.Items = append(snapshot.Items, protocolItemInstanceState(instance))
	}
	return snapshot, nil
}

// buildEquipmentInstanceSnapshot is the Protocol v28 complete unique-instance equipment view.
// Low-tier archetype-only occupants remain represented by EquipmentSnapshot; an empty Slots set is
// still authoritative and clears stale unique-equipment presentation on the Client.
func buildEquipmentInstanceSnapshot(inv *inventory.Inventory) (protocol.EquipmentInstanceSnapshot, error) {
	snapshot := protocol.EquipmentInstanceSnapshot{Revision: inv.EquipmentRevision()}
	if instance, ok := inv.MainHandInstance(); ok {
		if err := validateDurableEquipmentInstance(instance, "weapon", "main_hand"); err != nil {
			return protocol.EquipmentInstanceSnapshot{}, err
		}
		snapshot.Slots = append(snapshot.Slots, protocol.EquipmentInstanceSlotState{
			Slot: protocol.EquipmentSlotMainHand,
			Item: protocolItemInstanceState(instance),
		})
	}
	if instance, ok := inv.OffHandInstance(); ok {
		if err := validateDurableEquipmentInstance(instance, "shield", "off_hand"); err != nil {
			return protocol.EquipmentInstanceSnapshot{}, err
		}
		snapshot.Slots = append(snapshot.Slots, protocol.EquipmentInstanceSlotState{
			Slot: protocol.EquipmentSlotOffHand,
			Item: protocolItemInstanceState(instance),
		})
	}
	return snapshot, nil
}
