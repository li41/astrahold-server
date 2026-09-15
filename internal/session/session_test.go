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

func TestRegistryIndexesActiveSessionByEntity(t *testing.T) {
	registry := NewRegistry()
	s, err := New(10, 20, 30, NewQueueConnection(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(s); err != nil {
		t.Fatal(err)
	}
	got, ok := registry.GetByEntity(20)
	if !ok || got != s {
		t.Fatalf("GetByEntity() = (%p, %t), want (%p, true)", got, ok, s)
	}
	if _, err := registry.Remove(s.ID); err != nil {
		t.Fatal(err)
	}
	if got, ok := registry.GetByEntity(20); ok || got != nil {
		t.Fatalf("removed entity lookup = (%p, %t), want (nil, false)", got, ok)
	}
}

func TestRegistryEntityIndexPreservesAddFirstReplacement(t *testing.T) {
	registry := NewRegistry()
	oldSession, err := New(10, 20, 30, NewQueueConnection(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := New(11, 20, 30, NewQueueConnection(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(oldSession); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(replacement); err != nil {
		t.Fatal(err)
	}
	if got, ok := registry.GetByEntity(20); !ok || got != replacement {
		t.Fatalf("replacement lookup = (%p, %t), want (%p, true)", got, ok, replacement)
	}
	if _, err := registry.Remove(oldSession.ID); err != nil {
		t.Fatal(err)
	}
	if got, ok := registry.GetByEntity(20); !ok || got != replacement {
		t.Fatalf("lookup after old removal = (%p, %t), want (%p, true)", got, ok, replacement)
	}
	if _, err := registry.Remove(replacement.ID); err != nil {
		t.Fatal(err)
	}
	if got, ok := registry.GetByEntity(20); ok || got != nil {
		t.Fatalf("lookup after replacement removal = (%p, %t), want (nil, false)", got, ok)
	}
}
