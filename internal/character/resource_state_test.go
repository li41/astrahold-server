package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateAcceptsEmptyAndKnownResourceIDs(t *testing.T) {
	cases := []struct {
		id      actionresource.ID
		wantMax uint32
	}{
		{id: actionresource.Empty, wantMax: 0},
		{id: actionresource.Resolve, wantMax: 100},
		{id: actionresource.Momentum, wantMax: 100},
		{id: actionresource.HuntMomentum, wantMax: 100},
		{id: actionresource.StarHeat, wantMax: 100},
		{id: actionresource.OathSeal, wantMax: 3},
	}
	for index, tc := range cases {
		service, err := NewServiceWithResources(1000, 100)
		if err != nil { t.Fatal(err) }
		entityID := world.EntityID(index + 1)
		state := State{EntityID: entityID, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100, ActionResourceID: tc.id}
		if err := service.RegisterState(state); err != nil { t.Fatalf("resource %q rejected: %v", tc.id, err) }
		got, ok := service.State(entityID)
		resource := got.ActionResource()
		if !ok || resource.ID != tc.id || resource.Max != tc.wantMax {
			t.Fatalf("resource %q state=%#v ok=%v want max=%d", tc.id, resource, ok, tc.wantMax)
		}
	}
}

func TestRegisterStateRejectsUnknownOrInconsistentResourceState(t *testing.T) {
	invalid := []State{
		{EntityID: 1, HP: 1000, MaxHP: 1000, ActionResourceID: actionresource.ID("focus")},
		{EntityID: 2, HP: 1000, MaxHP: 1000, ActionResourceID: actionresource.Resolve, MaxActionResource: 50},
		{EntityID: 3, HP: 1000, MaxHP: 1000, ActionResourceID: actionresource.Resolve, MaxActionResource: 100, ActionResourceProgress: 1},
		{EntityID: 4, HP: 1000, MaxHP: 1000, ActionResourceID: actionresource.OathSeal, MaxActionResource: 3, ActionResourceProgress: 100},
		{EntityID: 5, HP: 1000, MaxHP: 1000, ActionResourceID: actionresource.OathSeal, ActionResourceCurrent: 3, MaxActionResource: 3, ActionResourceProgress: 1},
	}
	for _, state := range invalid {
		service, err := NewServiceWithResources(1000, 100)
		if err != nil { t.Fatal(err) }
		if err := service.RegisterState(state); err != ErrInvalidState {
			t.Fatalf("state=%#v err=%v want=%v", state, err, ErrInvalidState)
		}
	}
}
