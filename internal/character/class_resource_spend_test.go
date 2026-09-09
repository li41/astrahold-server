package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSpendClassResourceConsumesExactAmountAndRejectsInsufficientWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 51
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Breaker, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GainClassResource(entityID, classresource.Momentum, 30); err != nil {
		t.Fatal(err)
	}

	state, err := service.SpendClassResource(entityID, classresource.Momentum, 20)
	if err != nil {
		t.Fatal(err)
	}
	if state.ClassResource != 10 {
		t.Fatalf("momentum = %d, want 10", state.ClassResource)
	}

	state, err = service.SpendClassResource(entityID, classresource.Momentum, 20)
	if !errors.Is(err, ErrInsufficientResource) {
		t.Fatalf("error = %v, want ErrInsufficientResource", err)
	}
	if state.ClassResource != 10 {
		t.Fatalf("insufficient spend returned momentum = %d, want 10", state.ClassResource)
	}
	stored, _ := service.State(entityID)
	if stored.ClassResource != 10 {
		t.Fatalf("insufficient spend mutated momentum = %d, want 10", stored.ClassResource)
	}
}

func TestSpendClassResourceRejectsResourceMismatchWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 52
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Breaker, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GainClassResource(entityID, classresource.Momentum, 30); err != nil {
		t.Fatal(err)
	}

	state, err := service.SpendClassResource(entityID, classresource.Resolve, 20)
	if !errors.Is(err, classresource.ErrResourceMismatch) {
		t.Fatalf("error = %v, want ErrResourceMismatch", err)
	}
	if state.ClassResource != 30 {
		t.Fatalf("mismatch returned momentum = %d, want 30", state.ClassResource)
	}
	stored, _ := service.State(entityID)
	if stored.ClassResource != 30 {
		t.Fatalf("mismatch mutated momentum = %d, want 30", stored.ClassResource)
	}
}
