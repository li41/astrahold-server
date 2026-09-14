package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStatePreservesPrimaryStats(t *testing.T) {
	service, err := NewServiceWithResources(100, 50)
	if err != nil { t.Fatal(err) }
	want := characterstats.Primary{Strength: 19, Agility: 23, Constitution: 14, Intelligence: 17, Spirit: 16, Charisma: 15}
	if err := service.RegisterState(State{
		EntityID: 1,
		HP: 100, MaxHP: 100,
		MP: 50, MaxMP: 50,
		PrimaryStats: want,
	}); err != nil {
		t.Fatal(err)
	}
	state, ok := service.State(world.EntityID(1))
	if !ok { t.Fatal("missing character state") }
	if state.PrimaryStats != want { t.Fatalf("got=%#v want=%#v", state.PrimaryStats, want) }
}

func TestRegisterDefaultsToFormalNeutralPrimaryStats(t *testing.T) {
	service, err := NewServiceWithResources(100, 50)
	if err != nil { t.Fatal(err) }
	if err := service.Register(world.EntityID(2)); err != nil { t.Fatal(err) }
	state, ok := service.State(world.EntityID(2))
	if !ok { t.Fatal("missing character state") }
	if state.PrimaryStats != characterstats.DefaultPrimary() { t.Fatalf("got=%#v", state.PrimaryStats) }
}

func TestRegisterStateNormalizesLegacyAbsentPrimaryStats(t *testing.T) {
	service, err := NewServiceWithResources(100, 50)
	if err != nil { t.Fatal(err) }
	if err := service.RegisterState(State{EntityID: 3, HP: 100, MaxHP: 100, MP: 50, MaxMP: 50}); err != nil { t.Fatal(err) }
	state, ok := service.State(world.EntityID(3))
	if !ok { t.Fatal("missing character state") }
	if state.PrimaryStats != characterstats.DefaultPrimary() { t.Fatalf("got=%#v", state.PrimaryStats) }
}

func TestRegisterStateRejectsPartiallyInvalidPrimaryStats(t *testing.T) {
	service, err := NewServiceWithResources(100, 50)
	if err != nil { t.Fatal(err) }
	invalid := characterstats.DefaultPrimary()
	invalid.Charisma = 9
	if err := service.RegisterState(State{EntityID: 4, HP: 100, MaxHP: 100, MP: 50, MaxMP: 50, PrimaryStats: invalid}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("err=%v", err)
	}
}