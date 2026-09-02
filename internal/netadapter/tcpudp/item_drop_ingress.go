package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedPickupSink interface {
	EnqueueFencedPickupItem(worldruntime.SessionOwnershipFence, uint32, protocol.ClientPickupItem) error
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
