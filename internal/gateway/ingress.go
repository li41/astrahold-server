// Package gateway 將不可信的網路協定輸入轉成受控的 World Runtime command。
package gateway

import (
	"errors"

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

type CombatActionCommandSink interface {
	EnqueueCombatAction(session.ID, uint32, protocol.ClientCombatAction) error
}

type Ingress struct {
	sink MoveCommandSink
}

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
	case protocol.ClientCombatAction:
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		if !message.Valid() {
			return ErrInvalidClientEnvelope
		}
		return g.enqueueCombatAction(sessionID, envelope.Sequence, message)
	case *protocol.ClientCombatAction:
		if message == nil || !message.Valid() {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueueCombatAction(sessionID, envelope.Sequence, *message)
	default:
		return ErrUnsupportedClientMessage
	}
}

func (g *Ingress) enqueueCombatAction(sessionID session.ID, sequence uint32, action protocol.ClientCombatAction) error {
	sink, ok := g.sink.(CombatActionCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueueCombatAction(sessionID, sequence, action)
}
