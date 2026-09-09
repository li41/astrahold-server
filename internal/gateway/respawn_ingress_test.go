package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type respawnFakeSink struct {
	sessionID session.ID
	sequence  uint32
}

func (f *respawnFakeSink) EnqueueMove(session.ID, uint32, protocol.ClientMoveInput) error { return nil }
func (f *respawnFakeSink) EnqueueRespawnRequest(id session.ID, sequence uint32, _ protocol.ClientRespawnRequest) error {
	f.sessionID = id
	f.sequence = sequence
	return nil
}

func TestIngressRoutesReliableRespawnRequest(t *testing.T) {
	sink := &respawnFakeSink{}
	ingress := NewIngress(sink)
	if err := ingress.Handle(7, protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 41,
		Message:  protocol.ClientRespawnRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 41 {
		t.Fatalf("session=%d sequence=%d", sink.sessionID, sink.sequence)
	}
}

func TestIngressRejectsRespawnRequestOnRealtime(t *testing.T) {
	ingress := NewIngress(&respawnFakeSink{})
	err := ingress.Handle(7, protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 1,
		Message:  protocol.ClientRespawnRequest{},
	})
	if !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("err=%v want ErrInvalidClientDelivery", err)
	}
}

func TestIngressRejectsNilRespawnRequest(t *testing.T) {
	ingress := NewIngress(&respawnFakeSink{})
	var request *protocol.ClientRespawnRequest
	err := ingress.Handle(7, protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 1,
		Message:  request,
	})
	if !errors.Is(err, ErrInvalidClientEnvelope) {
		t.Fatalf("err=%v want ErrInvalidClientEnvelope", err)
	}
}
