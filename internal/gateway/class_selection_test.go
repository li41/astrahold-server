package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestIngressRejectsRetiredInitialClassSelection(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	for _, message := range []protocol.Message{
		protocol.ClientInitialClassSelection{ClassID: "class_ranger"},
		&protocol.ClientInitialClassSelection{ClassID: "class_ranger"},
	} {
		err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: message})
		if !errors.Is(err, ErrUnsupportedClientMessage) {
			t.Fatalf("message=%T err=%v, want ErrUnsupportedClientMessage", message, err)
		}
	}
}
