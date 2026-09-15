package worldruntime

import (
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
)

func inventoryEquipmentSlot(slot protocol.EquipmentSlot) (inventory.EquipmentSlot, bool) {
	switch slot {
	case protocol.EquipmentSlotMainHand: return inventory.SlotMainHand, true
	case protocol.EquipmentSlotOffHand: return inventory.SlotOffHand, true
	case protocol.EquipmentSlotHelmet: return inventory.SlotHelmet, true
	case protocol.EquipmentSlotChest: return inventory.SlotChest, true
	case protocol.EquipmentSlotGloves: return inventory.SlotGloves, true
	case protocol.EquipmentSlotLegs: return inventory.SlotLegs, true
	case protocol.EquipmentSlotBoots: return inventory.SlotBoots, true
	default: return "", false
	}
}

func protocolEquipmentSlot(slot inventory.EquipmentSlot) (protocol.EquipmentSlot, bool) {
	switch slot {
	case inventory.SlotMainHand: return protocol.EquipmentSlotMainHand, true
	case inventory.SlotOffHand: return protocol.EquipmentSlotOffHand, true
	case inventory.SlotHelmet: return protocol.EquipmentSlotHelmet, true
	case inventory.SlotChest: return protocol.EquipmentSlotChest, true
	case inventory.SlotGloves: return protocol.EquipmentSlotGloves, true
	case inventory.SlotLegs: return protocol.EquipmentSlotLegs, true
	case inventory.SlotBoots: return protocol.EquipmentSlotBoots, true
	default: return "", false
	}
}

func catalogEquipmentSlot(slot inventory.EquipmentSlot) (equipmentcatalog.Slot, bool) {
	switch slot {
	case inventory.SlotMainHand: return equipmentcatalog.SlotMainHand, true
	case inventory.SlotOffHand: return equipmentcatalog.SlotOffHand, true
	case inventory.SlotHelmet: return equipmentcatalog.SlotHelmet, true
	case inventory.SlotChest: return equipmentcatalog.SlotChest, true
	case inventory.SlotGloves: return equipmentcatalog.SlotGloves, true
	case inventory.SlotLegs: return equipmentcatalog.SlotLegs, true
	case inventory.SlotBoots: return equipmentcatalog.SlotBoots, true
	default: return "", false
	}
}

func expectedEquipmentKind(slot inventory.EquipmentSlot) (equipmentcatalog.Kind, bool) {
	switch slot {
	case inventory.SlotMainHand: return equipmentcatalog.KindWeapon, true
	case inventory.SlotOffHand: return equipmentcatalog.KindShield, true
	case inventory.SlotHelmet, inventory.SlotChest, inventory.SlotGloves, inventory.SlotLegs, inventory.SlotBoots: return equipmentcatalog.KindArmor, true
	default: return "", false
	}
}
