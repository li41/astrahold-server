package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedRespawnSink interface {
	EnqueueFencedRespawnRequest(worldruntime.SessionOwnershipFence, uint32, protocol.ClientRespawnRequest) error
}

func (s peerCommandSink) EnqueueRespawnRequest(id session.ID, sequence uint32, intent protocol.ClientRespawnRequest) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedRespawnSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedRespawnRequest(s.ownership, sequence, intent)
}
