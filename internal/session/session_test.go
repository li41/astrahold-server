package session

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestQueueConnectionBackpressureIsNonBlocking(t *testing.T) {
	c := NewQueueConnection(1, 1)
	env := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.WorldSnapshot{}}
	if err := c.TrySend(env); err != nil {
		t.Fatal(err)
	}
	if err := c.TrySend(env); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected backpressure, got %v", err)
	}
}

func TestInputSequenceIsSessionScoped(t *testing.T) {
	conn := NewQueueConnection(1, 1)
	s, err := New(10, 20, 30, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateInputSequence(1); err != nil {
		t.Fatal(err)
	}
	s.MarkProcessedInput(1)
	if err := s.ValidateInputSequence(1); !errors.Is(err, ErrStaleInput) {
		t.Fatalf("expected stale input, got %v", err)
	}

	reconnect, err := New(11, 20, 30, NewQueueConnection(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := reconnect.ValidateInputSequence(1); err != nil {
		t.Fatalf("new session should accept sequence 1: %v", err)
	}
}
