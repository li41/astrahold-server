package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesStarHeat(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 48
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.StarHeat, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	resource := state.ActionResource()
	if resource.ID != actionresource.StarHeat || resource.Current != 0 || resource.Max != 100 {
		t.Fatalf("resource = %q %d/%d, want star_heat 0/100", resource.ID, resource.Current, resource.Max)
	}
}

func TestStarHeatResourceGainClamps(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 49
	if err := service.RegisterState(State{EntityID: entityID, ActionResourceID: actionresource.StarHeat, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := service.GainActionResource(entityID, actionresource.StarHeat, 8); err != nil { t.Fatal(err) }
	if _, err := service.GainActionResource(entityID, actionresource.StarHeat, 200); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	resource := state.ActionResource()
	if !ok || resource.ID != actionresource.StarHeat || resource.Current != 100 || resource.Max != 100 {
		t.Fatalf("state=%+v ok=%v, want star_heat 100/100", resource, ok)
	}
}
