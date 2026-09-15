package status

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestStoreApplyRefreshExpiryRemoveAndClearEntity(t *testing.T) {
	store := NewStore()
	const (
		entityA world.EntityID = 10
		entityB world.EntityID = 11
		fortify ID = "self_damage_reduction"
		other ID = "other_effect"
	)

	store.Apply(entityA, fortify, 45, 62)
	if got, ok := store.Value(entityA, fortify, 2); !ok || got != 45 {
		t.Fatalf("initial value=(%d,%v), want (45,true)", got, ok)
	}

	store.Apply(entityA, fortify, 30, 80)
	if got, ok := store.Value(entityA, fortify, 79); !ok || got != 30 {
		t.Fatalf("refreshed value=(%d,%v), want (30,true)", got, ok)
	}
	if got, ok := store.Value(entityA, fortify, 80); ok || got != 0 {
		t.Fatalf("expired value=(%d,%v), want (0,false)", got, ok)
	}

	store.Apply(entityA, fortify, 45, 100)
	store.Remove(entityA, fortify)
	if _, ok := store.Value(entityA, fortify, 90); ok {
		t.Fatal("removed effect still active")
	}

	store.Apply(entityA, fortify, 45, 100)
	store.Apply(entityA, other, 1, 100)
	store.Apply(entityB, fortify, 20, 100)
	store.ClearEntity(entityA)
	if _, ok := store.Value(entityA, fortify, 90); ok {
		t.Fatal("entityA fortify survived ClearEntity")
	}
	if _, ok := store.Value(entityA, other, 90); ok {
		t.Fatal("entityA other effect survived ClearEntity")
	}
	if got, ok := store.Value(entityB, fortify, 90); !ok || got != 20 {
		t.Fatalf("entityB value=(%d,%v), want (20,true)", got, ok)
	}
}

func TestDeadlineSaturates(t *testing.T) {
	max := ^uint64(0)
	if got := Deadline(max-2, 3); got != max {
		t.Fatalf("overflow deadline=%d want=%d", got, max)
	}
	if got := Deadline(2, 60); got != 62 {
		t.Fatalf("normal deadline=%d want=62", got)
	}
}
