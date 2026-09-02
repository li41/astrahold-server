package protocol

// InventorySnapshot is a complete, reliable Server-authoritative inventory view for the owning session.
// Item presentation metadata is resolved client-side from stable ArchetypeID; Quantity is gameplay truth.
const MessageInventorySnapshot MessageType = 110

type InventoryItemStack struct {
	ArchetypeID string
	Quantity    uint32
}

type InventorySnapshot struct {
	Revision uint64
	Items    []InventoryItemStack
}

func (InventorySnapshot) Type() MessageType { return MessageInventorySnapshot }
