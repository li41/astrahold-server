package character

import (
	"errors"
	"testing"
)

func TestRestoreAliveFullRestoresHPAndMP(t *testing.T) {
	service, err := NewServiceWithResources(100, 60)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterState(State{EntityID: 7, HP: 25, MaxHP: 100, MP: 10, MaxMP: 60}); err != nil {
		t.Fatal(err)
	}

	state, err := service.RestoreAliveFull(7)
	if err != nil {
		t.Fatal(err)
	}
	if state.HP != 100 || state.MP != 60 || state.Defeated {
		t.Fatalf("state=%#v; want full alive resources", state)
	}
}

func TestRestoreAliveFullDoesNotReviveDefeatedCombatant(t *testing.T) {
	service, err := NewServiceWithResources(100, 60)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterState(State{EntityID: 7, HP: 0, MaxHP: 100, MP: 10, MaxMP: 60, Defeated: true}); err != nil {
		t.Fatal(err)
	}

	state, err := service.RestoreAliveFull(7)
	if !errors.Is(err, ErrCharacterDefeated) {
		t.Fatalf("err=%v; want ErrCharacterDefeated", err)
	}
	if state.HP != 0 || state.MP != 10 || !state.Defeated {
		t.Fatalf("state=%#v; defeated state must remain unchanged", state)
	}
}
