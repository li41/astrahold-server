package browserws

import (
	"github.com/li41/astrahold-server/internal/gateway"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

// ownedCommandSink binds one trusted BrowserWS peer to the immutable ownership fence returned by
// its committed join. Ephemeral BrowserWS peers continue to use the shared unfenced ingress because
// they do not own a durable CharacterID and therefore have no transferable ownership epoch.
type ownedCommandSink struct {
	runtime   RuntimeSink
	ownership worldruntime.SessionOwnershipFence
}

type fencedEquipmentSink interface {
	EnqueueFencedEquipmentCommand(worldruntime.SessionOwnershipFence, uint32, protocol.ClientEquipmentCommand) error
}

type fencedEquipmentInstanceSink interface {
	EnqueueFencedEquipmentInstanceCommand(worldruntime.SessionOwnershipFence, uint32, protocol.ClientEquipmentInstanceCommand) error
}

type fencedPickupSink interface {
	EnqueueFencedPickupItem(worldruntime.SessionOwnershipFence, uint32, protocol.ClientPickupItem) error
}

type fencedUseItemSink interface {
	EnqueueFencedUseItem(worldruntime.SessionOwnershipFence, uint32, protocol.ClientUseItem) error
}

type fencedNPCSink interface {
	EnqueueFencedInteractNPC(worldruntime.SessionOwnershipFence, uint32, protocol.ClientInteractNPC) error
}

type fencedShopSink interface {
	EnqueueFencedShopCommand(worldruntime.SessionOwnershipFence, uint32, protocol.ClientShopCommand) error
}

type fencedRespawnSink interface {
	EnqueueFencedRespawnRequest(worldruntime.SessionOwnershipFence, uint32, protocol.ClientRespawnRequest) error
}

func (s ownedCommandSink) validSession(id session.ID) bool {
	return s.ownership.Valid() && id == s.ownership.SessionID
}

func (s ownedCommandSink) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return s.runtime.EnqueueFencedMove(s.ownership, sequence, input)
}

func (s ownedCommandSink) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	return s.runtime.EnqueueFencedUseAction(s.ownership, sequence, action)
}

func (s ownedCommandSink) EnqueueEquipmentCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentCommand) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedEquipmentSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedEquipmentCommand(s.ownership, sequence, command)
}

func (s ownedCommandSink) EnqueueEquipmentInstanceCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentInstanceCommand) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedEquipmentInstanceSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedEquipmentInstanceCommand(s.ownership, sequence, command)
}

func (s ownedCommandSink) EnqueuePickupItem(id session.ID, sequence uint32, intent protocol.ClientPickupItem) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedPickupSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedPickupItem(s.ownership, sequence, intent)
}

func (s ownedCommandSink) EnqueueUseItem(id session.ID, sequence uint32, intent protocol.ClientUseItem) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedUseItemSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedUseItem(s.ownership, sequence, intent)
}

func (s ownedCommandSink) EnqueueInteractNPC(id session.ID, sequence uint32, intent protocol.ClientInteractNPC) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedNPCSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedInteractNPC(s.ownership, sequence, intent)
}

func (s ownedCommandSink) EnqueueShopCommand(id session.ID, sequence uint32, intent protocol.ClientShopCommand) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedShopSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedShopCommand(s.ownership, sequence, intent)
}

func (s ownedCommandSink) EnqueueRespawnRequest(id session.ID, sequence uint32, intent protocol.ClientRespawnRequest) error {
	if !s.validSession(id) {
		return worldruntime.ErrCharacterOwnershipFenceInvalid
	}
	sink, ok := s.runtime.(fencedRespawnSink)
	if !ok {
		return gateway.ErrUnsupportedClientMessage
	}
	return sink.EnqueueFencedRespawnRequest(s.ownership, sequence, intent)
}

var (
	_ gateway.MoveCommandSink              = ownedCommandSink{}
	_ gateway.ActionCommandSink            = ownedCommandSink{}
	_ gateway.EquipmentCommandSink         = ownedCommandSink{}
	_ gateway.EquipmentInstanceCommandSink = ownedCommandSink{}
	_ gateway.PickupCommandSink            = ownedCommandSink{}
	_ gateway.ItemUseCommandSink           = ownedCommandSink{}
	_ gateway.NPCCommandSink               = ownedCommandSink{}
	_ gateway.ShopCommandSink              = ownedCommandSink{}
	_ gateway.RespawnCommandSink           = ownedCommandSink{}
)
