package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedNPCSink interface {
	EnqueueFencedInteractNPC(worldruntime.SessionOwnershipFence, uint32, protocol.ClientInteractNPC) error
}

func (s peerCommandSink) EnqueueInteractNPC(id session.ID, sequence uint32, intent protocol.ClientInteractNPC) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedNPCSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedInteractNPC(s.ownership, sequence, intent)
}
