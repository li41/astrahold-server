package protocol

// InventorySnapshot is a complete, reliable Server-authoritative inventory view for the owning session.
// Item presentation metadata is resolved client-side from stable ArchetypeID; Quantity and carry
// weight are gameplay truth. MaxCarryWeight == 0 means the Server has no active weight limit.
const MessageInventorySnapshot MessageType = 110

type InventoryItemStack struct {
	ArchetypeID string
	Quantity    uint32
}

type InventorySnapshot struct {
	Revision           uint64
	CurrentCarryWeight uint64
	MaxCarryWeight     uint64
	Items              []InventoryItemStack
}

func (InventorySnapshot) Type() MessageType { return MessageInventorySnapshot }
