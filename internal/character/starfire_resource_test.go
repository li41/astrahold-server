package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesStarHeat(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 48
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: classresource.StarHeat, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	if state.ClassResourceID != classresource.StarHeat || state.ClassResource != 0 || state.MaxClassResource != 100 {
		t.Fatalf("resource = %q %d/%d, want star_heat 0/100", state.ClassResourceID, state.ClassResource, state.MaxClassResource)
	}
}

func TestStarHeatResourceGainClamps(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 49
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: classresource.StarHeat, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := service.GainClassResource(entityID, classresource.StarHeat, 8); err != nil { t.Fatal(err) }
	if _, err := service.GainClassResource(entityID, classresource.StarHeat, 200); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok || state.ClassResourceID != classresource.StarHeat || state.ClassResource != 100 || state.MaxClassResource != 100 {
		t.Fatalf("state=%+v ok=%v, want star_heat 100/100", state, ok)
	}
}
