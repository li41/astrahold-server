package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedShopSink interface {
	EnqueueFencedShopCommand(worldruntime.SessionOwnershipFence, uint32, protocol.ClientShopCommand) error
}

func (s peerCommandSink) EnqueueShopCommand(id session.ID, sequence uint32, intent protocol.ClientShopCommand) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedShopSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedShopCommand(s.ownership, sequence, intent)
}
