package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesOathguardResolve(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 41
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathguard, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok {
		t.Fatal("state missing")
	}
	if state.ClassResourceID != classresource.Resolve || state.ClassResource != 0 || state.MaxClassResource != 100 {
		t.Fatalf("resource = %q %d/%d, want resolve 0/100", state.ClassResourceID, state.ClassResource, state.MaxClassResource)
	}
}

func TestRegisterStateInitializesBreakerMomentum(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 45
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Breaker, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok {
		t.Fatal("state missing")
	}
	if state.ClassResourceID != classresource.Momentum || state.ClassResource != 0 || state.MaxClassResource != 100 {
		t.Fatalf("resource = %q %d/%d, want momentum 0/100", state.ClassResourceID, state.ClassResource, state.MaxClassResource)
	}
}

func TestLegacyOathguardResourceGainClamps(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 42
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathguard, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok {
		t.Fatal("state missing")
	}
	if state.ClassResourceID != classresource.Resolve || state.MaxClassResource != 100 {
		t.Fatalf("legacy resource = %q max=%d", state.ClassResourceID, state.MaxClassResource)
	}
	state, err = service.GainClassResource(entityID, classresource.Resolve, 8)
	if err != nil {
		t.Fatal(err)
	}
	if state.ClassResource != 8 {
		t.Fatalf("resolve = %d, want 8", state.ClassResource)
	}
	state, err = service.GainClassResource(entityID, classresource.Resolve, 200)
	if err != nil {
		t.Fatal(err)
	}
	if state.ClassResource != 100 {
		t.Fatalf("resolve = %d, want clamp 100", state.ClassResource)
	}
}

func TestClassResourceRejectsMismatchWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 43
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathguard, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	_, err = service.GainClassResource(entityID, classresource.ID("focus"), 8)
	if !errors.Is(err, classresource.ErrResourceMismatch) {
		t.Fatalf("error = %v, want ErrResourceMismatch", err)
	}
	state, _ := service.State(entityID)
	if state.ClassResource != 0 {
		t.Fatalf("resolve mutated to %d", state.ClassResource)
	}
}

func TestClassWithoutRuntimeResourceHasNone(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 44
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Shadowblade, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, _ := service.State(entityID)
	if state.ClassResourceID != classresource.Empty || state.ClassResource != 0 || state.MaxClassResource != 0 || state.ClassResourceProgress != 0 {
		t.Fatalf("unexpected shadowblade resource = %q %d/%d progress=%d", state.ClassResourceID, state.ClassResource, state.MaxClassResource, state.ClassResourceProgress)
	}
}
