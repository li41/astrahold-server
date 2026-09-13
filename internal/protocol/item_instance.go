package protocol

// These message IDs stage the next unique-equipment contract while protocol.Version remains v27.
// Runtime/adapters must not emit or accept them until the formal version cutover is coordinated
// with the Client. Keeping the IDs and payload semantics authored now prevents presentation work
// from inventing a parallel item-instance contract.
const (
	MessageClientEquipmentInstanceCommand MessageType = 119
	MessageInventoryInstanceSnapshot       MessageType = 120
	MessageEquipmentInstanceSnapshot       MessageType = 121
)

type ItemAffixState struct {
	AffixID   string
	Strength  uint8
	Value     uint32
}

type ItemInstanceState struct {
	ItemInstanceID  string
	ItemArchetypeID string
	Affixes         []ItemAffixState
}

// ClientEquipmentInstanceCommand identifies one exact Server-authoritative equipment instance.
// Equip requires ItemInstanceID. Unequip deliberately carries no item identity because the slot's
// current authoritative occupant is the only item the Server may remove.
type ClientEquipmentInstanceCommand struct {
	Operation      EquipmentOperation
	Slot           EquipmentSlot
	ItemInstanceID string
}

func (ClientEquipmentInstanceCommand) Type() MessageType { return MessageClientEquipmentInstanceCommand }

// InventoryInstanceSnapshot supplements InventorySnapshot with non-stackable unique instances.
// Revision is the same authoritative inventory revision used by the existing stack snapshot.
type InventoryInstanceSnapshot struct {
	Revision uint64
	Items    []ItemInstanceState
}

func (InventoryInstanceSnapshot) Type() MessageType { return MessageInventoryInstanceSnapshot }

type EquipmentInstanceSlotState struct {
	Slot EquipmentSlot
	Item ItemInstanceState
}

// EquipmentInstanceSnapshot supplements EquipmentSnapshot with exact unique-instance occupants.
// Low-tier archetype-only equipment remains represented by the existing EquipmentSnapshot.
type EquipmentInstanceSnapshot struct {
	Revision uint64
	Slots    []EquipmentInstanceSlotState
}

func (EquipmentInstanceSnapshot) Type() MessageType { return MessageEquipmentInstanceSnapshot }
