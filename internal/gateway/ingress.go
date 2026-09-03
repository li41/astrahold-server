// Package gateway 將不可信的網路協定輸入轉成受控的 World Runtime command。
package gateway

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var (
	ErrInvalidClientEnvelope    = errors.New("gateway: invalid client envelope")
	ErrInvalidClientDelivery    = errors.New("gateway: invalid client delivery")
	ErrUnsupportedClientMessage = errors.New("gateway: unsupported client message")
)

type MoveCommandSink interface {
	EnqueueMove(session.ID, uint32, protocol.ClientMoveInput) error
}
type ActionCommandSink interface {
	EnqueueUseAction(session.ID, uint32, protocol.ClientUseAction) error
}
type EquipmentCommandSink interface {
	EnqueueEquipmentCommand(session.ID, uint32, protocol.ClientEquipmentCommand) error
}
type PickupCommandSink interface {
	EnqueuePickupItem(session.ID, uint32, protocol.ClientPickupItem) error
}
type NPCCommandSink interface {
	EnqueueInteractNPC(session.ID, uint32, protocol.ClientInteractNPC) error
}

type Ingress struct{ sink MoveCommandSink }

func NewIngress(sink MoveCommandSink) *Ingress {
	if sink == nil {
		panic("gateway: move command sink is required")
	}
	return &Ingress{sink: sink}
}

// Handle 驗證 client 可送的 message/delivery，再轉交 bounded runtime queue。
func (g *Ingress) Handle(sessionID session.ID, envelope protocol.Envelope) error {
	if sessionID == 0 || envelope.Sequence == 0 || envelope.Message == nil {
		return ErrInvalidClientEnvelope
	}

	switch message := envelope.Message.(type) {
	case protocol.ClientMoveInput:
		if envelope.Delivery != protocol.DeliveryRealtimeSequenced {
			return ErrInvalidClientDelivery
		}
		return g.sink.EnqueueMove(sessionID, envelope.Sequence, message)
	case *protocol.ClientMoveInput:
		if message == nil {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryRealtimeSequenced {
			return ErrInvalidClientDelivery
		}
		return g.sink.EnqueueMove(sessionID, envelope.Sequence, *message)
	case protocol.ClientUseAction:
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		if !validAction(message) {
			return ErrInvalidClientEnvelope
		}
		return g.enqueueUseAction(sessionID, envelope.Sequence, message)
	case *protocol.ClientUseAction:
		if message == nil || !validAction(*message) {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueueUseAction(sessionID, envelope.Sequence, *message)
	case protocol.ClientEquipmentCommand:
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		if !validEquipmentCommand(message) {
			return ErrInvalidClientEnvelope
		}
		return g.enqueueEquipmentCommand(sessionID, envelope.Sequence, message)
	case *protocol.ClientEquipmentCommand:
		if message == nil || !validEquipmentCommand(*message) {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueueEquipmentCommand(sessionID, envelope.Sequence, *message)
	case protocol.ClientPickupItem:
		if envelope.Delivery != protocol.DeliveryReliableOrdered || message.DropEntityID == 0 {
			if envelope.Delivery != protocol.DeliveryReliableOrdered {
				return ErrInvalidClientDelivery
			}
			return ErrInvalidClientEnvelope
		}
		return g.enqueuePickupItem(sessionID, envelope.Sequence, message)
	case *protocol.ClientPickupItem:
		if message == nil || message.DropEntityID == 0 {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueuePickupItem(sessionID, envelope.Sequence, *message)
	case protocol.ClientInteractNPC:
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		if message.NPCEntityID == 0 {
			return ErrInvalidClientEnvelope
		}
		return g.enqueueInteractNPC(sessionID, envelope.Sequence, message)
	case *protocol.ClientInteractNPC:
		if message == nil || message.NPCEntityID == 0 {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueueInteractNPC(sessionID, envelope.Sequence, *message)
	default:
		return ErrUnsupportedClientMessage
	}
}

func validAction(action protocol.ClientUseAction) bool {
	if action.ActionID == "" {
		return false
	}
	switch action.TargetKind {
	case protocol.ActionTargetGate, protocol.ActionTargetEntity:
		return action.TargetID != "" && action.TargetX == nil && action.TargetZ == nil
	case protocol.ActionTargetPoint:
		if action.TargetID != "" || action.TargetX == nil || action.TargetZ == nil {
			return false
		}
		return finiteFloat32(*action.TargetX) && finiteFloat32(*action.TargetZ)
	default:
		return false
	}
}

func validEquipmentCommand(command protocol.ClientEquipmentCommand) bool {
	if command.Slot != protocol.EquipmentSlotMainHand {
		return false
	}
	switch command.Operation {
	case protocol.EquipmentOperationEquip:
		return command.ItemArchetypeID != ""
	case protocol.EquipmentOperationUnequip:
		return command.ItemArchetypeID == ""
	default:
		return false
	}
}

func finiteFloat32(value float32) bool { return !float32NaN(value) && !float32Inf(value) }
func float32NaN(value float32) bool    { return math.IsNaN(float64(value)) }
func float32Inf(value float32) bool    { return math.IsInf(float64(value), 0) }

func (g *Ingress) enqueueUseAction(sessionID session.ID, sequence uint32, action protocol.ClientUseAction) error {
	sink, ok := g.sink.(ActionCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueueUseAction(sessionID, sequence, action)
}
func (g *Ingress) enqueueEquipmentCommand(sessionID session.ID, sequence uint32, command protocol.ClientEquipmentCommand) error {
	sink, ok := g.sink.(EquipmentCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueueEquipmentCommand(sessionID, sequence, command)
}
func (g *Ingress) enqueuePickupItem(sessionID session.ID, sequence uint32, intent protocol.ClientPickupItem) error {
	sink, ok := g.sink.(PickupCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueuePickupItem(sessionID, sequence, intent)
}
func (g *Ingress) enqueueInteractNPC(sessionID session.ID, sequence uint32, intent protocol.ClientInteractNPC) error {
	sink, ok := g.sink.(NPCCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueueInteractNPC(sessionID, sequence, intent)
}
