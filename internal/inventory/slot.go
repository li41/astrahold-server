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
)

var equipmentSlots = [...]EquipmentSlot{
	SlotMainHand,
	SlotOffHand,
	SlotHelmet,
	SlotChest,
	SlotGloves,
	SlotLegs,
	SlotBoots,
}

func EquipmentSlots() []EquipmentSlot {
	out := make([]EquipmentSlot, len(equipmentSlots))
	copy(out, equipmentSlots[:])
	return out
}

func ValidEquipmentSlot(slot EquipmentSlot) bool {
	switch slot {
	case SlotMainHand, SlotOffHand, SlotHelmet, SlotChest, SlotGloves, SlotLegs, SlotBoots:
		return true
	default:
		return false
	}
}
