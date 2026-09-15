package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesHuntMomentum(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 46
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.HuntMomentum, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	resource := state.ActionResource()
	if resource.ID != actionresource.HuntMomentum || resource.Current != 0 || resource.Max != 100 {
		t.Fatalf("resource = %q %d/%d, want hunt_momentum 0/100", resource.ID, resource.Current, resource.Max)
	}
}

func TestHuntMomentumResourceGain(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 47
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.HuntMomentum, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := service.GainActionResource(entityID, actionresource.HuntMomentum, 8); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	resource := state.ActionResource()
	if !ok || resource.ID != actionresource.HuntMomentum || resource.Current != 8 || resource.Max != 100 {
		t.Fatalf("state=%+v ok=%v, want hunt_momentum 8/100", resource, ok)
	}
}
