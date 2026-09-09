package tcpudp

import (
	"github.com/li41/astrahold-server/internal/gateway"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

// peerCommandSink binds one trusted transport peer to the immutable world-owner ownership
// fence returned by its committed join. Ephemeral peers keep using the legacy shared ingress.
type peerCommandSink struct {
	runtime   RuntimeSink
	ownership worldruntime.SessionOwnershipFence
}

type fencedInitialClassSelectionSink interface {
	EnqueueFencedInitialClassSelection(worldruntime.SessionOwnershipFence, uint32, protocol.ClientInitialClassSelection) error
}

func (s peerCommandSink) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return s.runtime.EnqueueFencedMove(s.ownership, sequence, input)
}

func (s peerCommandSink) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return s.runtime.EnqueueFencedUseAction(s.ownership, sequence, action)
}

func (s peerCommandSink) EnqueueInitialClassSelection(id session.ID, sequence uint32, intent protocol.ClientInitialClassSelection) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedInitialClassSelectionSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedInitialClassSelection(s.ownership, sequence, intent)
}
