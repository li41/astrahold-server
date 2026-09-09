package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedPickupSink interface {
	EnqueueFencedPickupItem(worldruntime.SessionOwnershipFence, uint32, protocol.ClientPickupItem) error
}

type fencedUseItemSink interface {
	EnqueueFencedUseItem(worldruntime.SessionOwnershipFence, uint32, protocol.ClientUseItem) error
}

func (s peerCommandSink) EnqueuePickupItem(id session.ID, sequence uint32, intent protocol.ClientPickupItem) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedPickupSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedPickupItem(s.ownership, sequence, intent)
}

func (s peerCommandSink) EnqueueUseItem(id session.ID, sequence uint32, intent protocol.ClientUseItem) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedUseItemSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedUseItem(s.ownership, sequence, intent)
}
