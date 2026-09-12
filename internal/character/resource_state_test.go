package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateAcceptsEmptyAndKnownResourceIDs(t *testing.T) {
	cases := []struct {
		id      classresource.ID
		wantMax uint32
	}{
		{id: classresource.Empty, wantMax: 0},
		{id: classresource.Resolve, wantMax: 100},
		{id: classresource.Momentum, wantMax: 100},
		{id: classresource.HuntMomentum, wantMax: 100},
		{id: classresource.StarHeat, wantMax: 100},
		{id: classresource.OathSeal, wantMax: 3},
	}
	for index, tc := range cases {
		service, err := NewServiceWithResources(1000, 100)
		if err != nil { t.Fatal(err) }
		entityID := world.EntityID(index + 1)
		state := State{EntityID: entityID, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100, ClassResourceID: tc.id}
		if err := service.RegisterState(state); err != nil { t.Fatalf("resource %q rejected: %v", tc.id, err) }
		got, ok := service.State(entityID)
		if !ok || got.ClassResourceID != tc.id || got.MaxClassResource != tc.wantMax {
			t.Fatalf("resource %q state=%#v ok=%v want max=%d", tc.id, got, ok, tc.wantMax)
		}
	}
}

func TestRegisterStateRejectsUnknownOrInconsistentResourceState(t *testing.T) {
	invalid := []State{
		{EntityID: 1, HP: 1000, MaxHP: 1000, ClassResourceID: classresource.ID("focus")},
		{EntityID: 2, HP: 1000, MaxHP: 1000, ClassResourceID: classresource.Resolve, MaxClassResource: 50},
		{EntityID: 3, HP: 1000, MaxHP: 1000, ClassResourceID: classresource.Resolve, MaxClassResource: 100, ClassResourceProgress: 1},
		{EntityID: 4, HP: 1000, MaxHP: 1000, ClassResourceID: classresource.OathSeal, MaxClassResource: 3, ClassResourceProgress: 100},
		{EntityID: 5, HP: 1000, MaxHP: 1000, ClassResourceID: classresource.OathSeal, ClassResource: 3, MaxClassResource: 3, ClassResourceProgress: 1},
	}
	for _, state := range invalid {
		service, err := NewServiceWithResources(1000, 100)
		if err != nil { t.Fatal(err) }
		if err := service.RegisterState(state); err != ErrInvalidState {
			t.Fatalf("state=%#v err=%v want=%v", state, err, ErrInvalidState)
		}
	}
}
