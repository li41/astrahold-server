package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesResolveDefinition(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 41
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.Resolve, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	resource := state.ActionResource()
	if resource.ID != actionresource.Resolve || resource.Current != 0 || resource.Max != 100 {
		t.Fatalf("resource = %q %d/%d, want resolve 0/100", resource.ID, resource.Current, resource.Max)
	}
}

func TestRegisterStateInitializesMomentumDefinition(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 45
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.Momentum, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	resource := state.ActionResource()
	if resource.ID != actionresource.Momentum || resource.Current != 0 || resource.Max != 100 {
		t.Fatalf("resource = %q %d/%d, want momentum 0/100", resource.ID, resource.Current, resource.Max)
	}
}

func TestActionResourceGainClamps(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 42
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.Resolve, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, err := service.GainActionResource(entityID, actionresource.Resolve, 8)
	if err != nil { t.Fatal(err) }
	if got := state.ActionResource().Current; got != 8 { t.Fatalf("resolve = %d, want 8", got) }
	state, err = service.GainActionResource(entityID, actionresource.Resolve, 200)
	if err != nil { t.Fatal(err) }
	if got := state.ActionResource().Current; got != 100 { t.Fatalf("resolve = %d, want clamp 100", got) }
}

func TestActionResourceRejectsMismatchWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 43
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.Resolve, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	_, err = service.GainActionResource(entityID, actionresource.ID("focus"), 8)
	if !errors.Is(err, actionresource.ErrResourceMismatch) { t.Fatalf("error = %v, want ErrResourceMismatch", err) }
	state, _ := service.State(entityID)
	if got := state.ActionResource().Current; got != 0 { t.Fatalf("resolve mutated to %d", got) }
}

func TestRegisterStateWithoutRuntimeResourceHasNone(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 44
	if err := service.RegisterState(State{EntityID: entityID, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, _ := service.State(entityID)
	resource := state.ActionResource()
	if resource.ID != actionresource.Empty || resource.Current != 0 || resource.Max != 0 || resource.Progress != 0 {
		t.Fatalf("unexpected resource = %q %d/%d progress=%d", resource.ID, resource.Current, resource.Max, resource.Progress)
	}
}
