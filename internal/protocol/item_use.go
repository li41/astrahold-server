package protocol

const MessageClientUseItem MessageType = 8

// ClientUseItem is intent only. The Server resolves the stable item archetype into gameplay
// effects, validates inventory ownership and character state, then mutates inventory/vitals.
type ClientUseItem struct {
	ItemArchetypeID string
}

func (ClientUseItem) Type() MessageType { return MessageClientUseItem }
