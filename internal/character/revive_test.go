package character

import (
	"errors"
	"testing"
)

func TestReviveFullRequiresDefeatedAndRestoresMaxHP(t *testing.T) {
	service, err := NewService(200)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(1); err != nil {
		t.Fatal(err)
	}

	if _, err := service.ReviveFull(1); !errors.Is(err, ErrCharacterNotDefeated) {
		t.Fatalf("alive revive err=%v", err)
	}
	if _, err := service.ReduceHP(1, 200); err != nil {
		t.Fatal(err)
	}
	state, err := service.ReviveFull(1)
	if err != nil {
		t.Fatal(err)
	}
	if state.HP != 200 || state.MaxHP != 200 || state.Defeated {
		t.Fatalf("revived state=%#v", state)
	}
}
