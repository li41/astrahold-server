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

const EquipmentSlotMainHand EquipmentSlot = "main_hand"

// ClientEquipmentCommand is intent only. The Server validates inventory ownership,
// slot legality and the resulting authoritative transaction.
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

// EquipmentSnapshot is the owning character's complete authoritative equipment view.
type EquipmentSnapshot struct {
	Revision uint64
	Slots    []EquipmentSlotState
}

func (EquipmentSnapshot) Type() MessageType { return MessageEquipmentSnapshot }
