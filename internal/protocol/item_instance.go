package protocol

// Protocol v28 activates exact unique-equipment identity and complete unique-instance snapshots.
// These IDs are stable wire contract and must not be reused for appearance or presentation metadata.
const (
	MessageClientEquipmentInstanceCommand MessageType = 119
	MessageInventoryInstanceSnapshot       MessageType = 120
	MessageEquipmentInstanceSnapshot       MessageType = 121
)

type ItemAffixState struct {
	AffixID  string
	Strength uint8
	Value    uint32
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
