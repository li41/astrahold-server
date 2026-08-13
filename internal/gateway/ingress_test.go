package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type fakeSink struct {
	sessionID session.ID
	sequence  uint32
	input     protocol.ClientMoveInput
	err       error
}

func (f *fakeSink) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	f.sessionID = id
	f.sequence = sequence
	f.input = input
	return f.err
}

func TestIngressUsesEnvelopeSequenceForMove(t *testing.T) {
	sink := &fakeSink{}
	ingress := NewIngress(sink)
	err := ingress.Handle(7, protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 42,
		Message:  protocol.ClientMoveInput{DirectionX: 1, DirectionZ: -0.25},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 42 {
		t.Fatalf("unexpected routing: session=%d sequence=%d", sink.sessionID, sink.sequence)
	}
}

func TestIngressRejectsReliableMove(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 1,
		Message:  protocol.ClientMoveInput{DirectionX: 1},
	})
	if !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("err=%v, want ErrInvalidClientDelivery", err)
	}
}

func TestIngressRejectsServerOnlyMessage(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 1,
		Message:  protocol.EntityDespawn{EntityID: 99},
	})
	if !errors.Is(err, ErrUnsupportedClientMessage) {
		t.Fatalf("err=%v, want ErrUnsupportedClientMessage", err)
	}
}
