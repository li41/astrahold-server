package protocol

const (
	MessageClientEquipmentCommand MessageType = 3
	MessageEquipmentSnapshot      MessageType = 111
)

type EquipmentOperation string

const (
	EquipmentOperationEquip   EquipmentOperation = "equip"
	EquipmentOperationUnequip EquipmentOperation = "unequip"
)

type EquipmentSlot string

const (
	EquipmentSlotMainHand EquipmentSlot = "main_hand"
	EquipmentSlotOffHand  EquipmentSlot = "off_hand"
	EquipmentSlotHelmet   EquipmentSlot = "helmet"
	EquipmentSlotChest    EquipmentSlot = "chest"
	EquipmentSlotGloves   EquipmentSlot = "gloves"
	EquipmentSlotLegs     EquipmentSlot = "legs"
	EquipmentSlotBoots    EquipmentSlot = "boots"
)

// EquipmentSlots returns the formal Protocol v29 equipment-slot order. The order is stable wire
// presentation policy only; gameplay legality remains Server-owned and is validated per item.
func EquipmentSlots() []EquipmentSlot {
	return []EquipmentSlot{
		EquipmentSlotMainHand,
		EquipmentSlotOffHand,
		EquipmentSlotHelmet,
		EquipmentSlotChest,
		EquipmentSlotGloves,
		EquipmentSlotLegs,
		EquipmentSlotBoots,
	}
}

type ClientEquipmentCommand struct {
	Operation       EquipmentOperation
	Slot            EquipmentSlot
	ItemArchetypeID string
}

func (ClientEquipmentCommand) Type() MessageType { return MessageClientEquipmentCommand }

type EquipmentSlotState struct {
	Slot            EquipmentSlot
	ItemArchetypeID string
}

type EquipmentSnapshot struct {
	Revision uint64
	Slots    []EquipmentSlotState
}

func (EquipmentSnapshot) Type() MessageType { return MessageEquipmentSnapshot }
