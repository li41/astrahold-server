package inventory

// EquipmentSlot is an internal authoritative equipment location. Wire protocol owns its own
// string type; worldruntime performs the narrow mapping at the boundary.
type EquipmentSlot string

const (
	SlotMainHand EquipmentSlot = "main_hand"
	SlotOffHand  EquipmentSlot = "off_hand"
	SlotHelmet   EquipmentSlot = "helmet"
	SlotChest    EquipmentSlot = "chest"
	SlotGloves   EquipmentSlot = "gloves"
	SlotLegs     EquipmentSlot = "legs"
	SlotBoots    EquipmentSlot = "boots"
	SlotNecklace EquipmentSlot = "necklace"
	SlotRing1    EquipmentSlot = "ring_1"
	SlotRing2    EquipmentSlot = "ring_2"
	SlotBelt     EquipmentSlot = "belt"
)

var equipmentSlots = [...]EquipmentSlot{
	SlotMainHand,
	SlotOffHand,
	SlotHelmet,
	SlotChest,
	SlotGloves,
	SlotLegs,
	SlotBoots,
	SlotNecklace,
	SlotRing1,
	SlotRing2,
	SlotBelt,
}

func EquipmentSlots() []EquipmentSlot {
	out := make([]EquipmentSlot, len(equipmentSlots))
	copy(out, equipmentSlots[:])
	return out
}

func ValidEquipmentSlot(slot EquipmentSlot) bool {
	switch slot {
	case SlotMainHand, SlotOffHand, SlotHelmet, SlotChest, SlotGloves, SlotLegs, SlotBoots,
		SlotNecklace, SlotRing1, SlotRing2, SlotBelt:
		return true
	default:
		return false
	}
}
