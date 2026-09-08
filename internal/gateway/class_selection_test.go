package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type classSelectionSink struct {
	fakeSink
	classIntent protocol.ClientInitialClassSelection
}

func (s *classSelectionSink) EnqueueInitialClassSelection(id session.ID, sequence uint32, intent protocol.ClientInitialClassSelection) error {
	s.sessionID = id
	s.sequence = sequence
	s.classIntent = intent
	return s.err
}

func TestIngressRoutesReliableInitialClassSelection(t *testing.T) {
	sink := &classSelectionSink{}
	ingress := NewIngress(sink)
	intent := protocol.ClientInitialClassSelection{ClassID: "class_ranger"}
	if err := ingress.Handle(7, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 12, Message: intent}); err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 12 || sink.classIntent != intent {
		t.Fatalf("unexpected class routing: session=%d sequence=%d intent=%#v", sink.sessionID, sink.sequence, sink.classIntent)
	}
}

func TestIngressLeavesUnknownClassForWorldOwnerRejection(t *testing.T) {
	sink := &classSelectionSink{}
	ingress := NewIngress(sink)
	intent := protocol.ClientInitialClassSelection{ClassID: "class_future_typo"}
	if err := ingress.Handle(3, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 4, Message: intent}); err != nil {
		t.Fatal(err)
	}
	if sink.classIntent.ClassID != intent.ClassID {
		t.Fatalf("gateway normalized class id: got=%q want=%q", sink.classIntent.ClassID, intent.ClassID)
	}
}

func TestIngressRejectsMalformedInitialClassSelection(t *testing.T) {
	ingress := NewIngress(&classSelectionSink{})
	if err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.ClientInitialClassSelection{ClassID: "   "}}); !errors.Is(err, ErrInvalidClientEnvelope) {
		t.Fatalf("whitespace class err=%v", err)
	}
	if err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 2, Message: protocol.ClientInitialClassSelection{ClassID: "class_ranger"}}); !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("realtime class err=%v", err)
	}
}

func TestIngressRejectsClassSelectionWhenSinkDoesNotSupportIt(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.ClientInitialClassSelection{ClassID: "class_ranger"}})
	if !errors.Is(err, ErrUnsupportedClientMessage) {
		t.Fatalf("err=%v, want ErrUnsupportedClientMessage", err)
	}
}
