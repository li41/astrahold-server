package tcpudp

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedEquipmentSink interface {
	EnqueueFencedEquipmentCommand(worldruntime.SessionOwnershipFence, uint32, protocol.ClientEquipmentCommand) error
}

func (s peerCommandSink) EnqueueEquipmentCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentCommand) error {
	if !s.ownership.Valid() || id != s.ownership.SessionID {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedEquipmentSink)
	if !ok {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return sink.EnqueueFencedEquipmentCommand(s.ownership, sequence, command)
}
