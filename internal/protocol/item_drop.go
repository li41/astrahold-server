package protocol

import "github.com/li41/astrahold-server/internal/world"

const MessageClientPickupItem MessageType = 4

// ClientPickupItem is intent only. The Server validates the drop, ownership,
// world distance/layer and inventory capacity before mutating authoritative state.
type ClientPickupItem struct {
	DropEntityID world.EntityID
}

func (ClientPickupItem) Type() MessageType { return MessageClientPickupItem }
