package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateInitializesRangerHuntMomentum(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 46
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Ranger, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok {
		t.Fatal("state missing")
	}
	if state.ClassResourceID != classresource.HuntMomentum || state.ClassResource != 0 || state.MaxClassResource != 100 {
		t.Fatalf("resource = %q %d/%d, want hunt_momentum 0/100", state.ClassResourceID, state.ClassResource, state.MaxClassResource)
	}
}

func TestLegacyRangerResourceGain(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 47
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Ranger, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GainClassResource(entityID, classresource.HuntMomentum, 8); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok || state.ClassID != classid.Ranger || state.ClassResourceID != classresource.HuntMomentum || state.ClassResource != 8 || state.MaxClassResource != 100 {
		t.Fatalf("state=%+v ok=%v, want legacy ranger hunt_momentum 8/100", state, ok)
	}
}
