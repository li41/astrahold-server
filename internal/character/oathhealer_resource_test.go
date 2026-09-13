package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestOathSealProgressConvertsEveryHundred(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 51
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: actionresource.OathSeal, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, ok := service.State(entityID)
	if !ok { t.Fatal("state missing") }
	resource := state.ActionResource()
	if resource.ID != actionresource.OathSeal || resource.Current != 0 || resource.Max != 3 || resource.Progress != 0 {
		t.Fatalf("initial oath seal state=%+v", resource)
	}

	for i := uint32(1); i <= 4; i++ {
		state, visibleChanged, err := service.GainActionResourceProgress(entityID, actionresource.OathSeal, 20)
		if err != nil { t.Fatal(err) }
		if visibleChanged { t.Fatalf("hit %d unexpectedly changed visible oath seals", i) }
		resource = state.ActionResource()
		if resource.Current != 0 || resource.Progress != i*20 { t.Fatalf("hit %d state=%+v", i, resource) }
	}
	state, visibleChanged, err := service.GainActionResourceProgress(entityID, actionresource.OathSeal, 20)
	if err != nil { t.Fatal(err) }
	resource = state.ActionResource()
	if !visibleChanged || resource.Current != 1 || resource.Progress != 0 {
		t.Fatalf("fifth hit state=%+v visible_changed=%v, want 1 seal progress 0", resource, visibleChanged)
	}
}

func TestOathSealProgressClampsAtThreeWithoutBankingOverflow(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 52
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: actionresource.OathSeal, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	state, visibleChanged, err := service.GainActionResourceProgress(entityID, actionresource.OathSeal, 360)
	if err != nil { t.Fatal(err) }
	resource := state.ActionResource()
	if !visibleChanged || resource.Current != 3 || resource.Progress != 0 {
		t.Fatalf("clamped state=%+v visible_changed=%v, want 3 seals progress 0", resource, visibleChanged)
	}
	state, visibleChanged, err = service.GainActionResourceProgress(entityID, actionresource.OathSeal, 20)
	if err != nil { t.Fatal(err) }
	resource = state.ActionResource()
	if visibleChanged || resource.Current != 3 || resource.Progress != 0 {
		t.Fatalf("full-cap gain changed state=%+v visible_changed=%v", resource, visibleChanged)
	}
}

func TestOathSealProgressRejectsWrongResourceWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil { t.Fatal(err) }
	const entityID world.EntityID = 53
	if err := service.RegisterState(State{EntityID: entityID, ClassResourceID: actionresource.OathSeal, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	_, _, err = service.GainActionResourceProgress(entityID, actionresource.Resolve, 20)
	if !errors.Is(err, actionresource.ErrResourceMismatch) { t.Fatalf("error=%v, want ErrResourceMismatch", err) }
	state, _ := service.State(entityID)
	resource := state.ActionResource()
	if resource.Current != 0 || resource.Progress != 0 { t.Fatalf("mismatch mutated state=%+v", resource) }
}
