package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSpendActionResourceConsumesExactAmountAndRejectsInsufficientWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 51
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: actionresource.Momentum, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := service.GainActionResource(entityID, actionresource.Momentum, 30); err != nil { t.Fatal(err) }

	state, err := service.SpendActionResource(entityID, actionresource.Momentum, 20)
	if err != nil { t.Fatal(err) }
	if got := state.ActionResource().Current; got != 10 { t.Fatalf("momentum = %d, want 10", got) }

	state, err = service.SpendActionResource(entityID, actionresource.Momentum, 20)
	if !errors.Is(err, ErrInsufficientResource) { t.Fatalf("error = %v, want ErrInsufficientResource", err) }
	if got := state.ActionResource().Current; got != 10 { t.Fatalf("insufficient spend returned momentum = %d, want 10", got) }
	stored, _ := service.State(entityID)
	if got := stored.ActionResource().Current; got != 10 { t.Fatalf("insufficient spend mutated momentum = %d, want 10", got) }
}

func TestSpendActionResourceRejectsResourceMismatchWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 52
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: actionresource.Momentum, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := service.GainActionResource(entityID, actionresource.Momentum, 30); err != nil { t.Fatal(err) }

	state, err := service.SpendActionResource(entityID, actionresource.Resolve, 20)
	if !errors.Is(err, actionresource.ErrResourceMismatch) { t.Fatalf("error = %v, want ErrResourceMismatch", err) }
	if got := state.ActionResource().Current; got != 30 { t.Fatalf("mismatch returned momentum = %d, want 30", got) }
	stored, _ := service.State(entityID)
	if got := stored.ActionResource().Current; got != 30 { t.Fatalf("mismatch mutated momentum = %d, want 30", got) }
}
