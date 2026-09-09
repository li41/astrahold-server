package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestOathhealerOathSealProgressConvertsEveryHundred(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 51
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathhealer, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(entityID)
	if !ok {
		t.Fatal("state missing")
	}
	if state.ClassResourceID != classresource.OathSeal || state.ClassResource != 0 || state.MaxClassResource != 3 || state.ClassResourceProgress != 0 {
		t.Fatalf("initial oath seal state=%+v", state)
	}

	for i := uint32(1); i <= 4; i++ {
		state, visibleChanged, err := service.GainClassResourceProgress(entityID, classresource.OathSeal, 20)
		if err != nil {
			t.Fatal(err)
		}
		if visibleChanged {
			t.Fatalf("hit %d unexpectedly changed visible oath seals", i)
		}
		if state.ClassResource != 0 || state.ClassResourceProgress != i*20 {
			t.Fatalf("hit %d state=%+v", i, state)
		}
	}
	state, visibleChanged, err := service.GainClassResourceProgress(entityID, classresource.OathSeal, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !visibleChanged || state.ClassResource != 1 || state.ClassResourceProgress != 0 {
		t.Fatalf("fifth hit state=%+v visible_changed=%v, want 1 seal progress 0", state, visibleChanged)
	}
}

func TestOathhealerOathSealProgressClampsAtThreeWithoutBankingOverflow(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 52
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathhealer, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	state, visibleChanged, err := service.GainClassResourceProgress(entityID, classresource.OathSeal, 360)
	if err != nil {
		t.Fatal(err)
	}
	if !visibleChanged || state.ClassResource != 3 || state.ClassResourceProgress != 0 {
		t.Fatalf("clamped state=%+v visible_changed=%v, want 3 seals progress 0", state, visibleChanged)
	}
	state, visibleChanged, err = service.GainClassResourceProgress(entityID, classresource.OathSeal, 20)
	if err != nil {
		t.Fatal(err)
	}
	if visibleChanged || state.ClassResource != 3 || state.ClassResourceProgress != 0 {
		t.Fatalf("full-cap gain changed state=%+v visible_changed=%v", state, visibleChanged)
	}
}

func TestOathSealProgressRejectsWrongResourceWithoutMutation(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	const entityID world.EntityID = 53
	if err := service.RegisterState(State{EntityID: entityID, ClassID: classid.Oathhealer, HP: 1000, MaxHP: 1000}); err != nil {
		t.Fatal(err)
	}
	_, _, err = service.GainClassResourceProgress(entityID, classresource.Resolve, 20)
	if !errors.Is(err, classresource.ErrResourceMismatch) {
		t.Fatalf("error=%v, want ErrResourceMismatch", err)
	}
	state, _ := service.State(entityID)
	if state.ClassResource != 0 || state.ClassResourceProgress != 0 {
		t.Fatalf("mismatch mutated state=%+v", state)
	}
}
