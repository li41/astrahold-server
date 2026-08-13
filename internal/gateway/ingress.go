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

type GateAttackCommandSink interface {
	EnqueueAttackGate(session.ID, uint32, string) error
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
	case protocol.ClientAttackGate:
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		if message.GateID == "" {
			return ErrInvalidClientEnvelope
		}
		return g.enqueueAttackGate(sessionID, envelope.Sequence, message.GateID)
	case *protocol.ClientAttackGate:
		if message == nil || message.GateID == "" {
			return ErrInvalidClientEnvelope
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			return ErrInvalidClientDelivery
		}
		return g.enqueueAttackGate(sessionID, envelope.Sequence, message.GateID)
	default:
		return ErrUnsupportedClientMessage
	}
}

func (g *Ingress) enqueueAttackGate(sessionID session.ID, sequence uint32, gateID string) error {
	sink, ok := g.sink.(GateAttackCommandSink)
	if !ok {
		return ErrUnsupportedClientMessage
	}
	return sink.EnqueueAttackGate(sessionID, sequence, gateID)
}
