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

func TestRevivePercentRestoresCeiledShareOfMaxHP(t *testing.T) {
	service, err := NewService(201)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(2); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReduceHP(2, 201); err != nil {
		t.Fatal(err)
	}
	state, err := service.RevivePercent(2, 30)
	if err != nil {
		t.Fatal(err)
	}
	if state.HP != 61 || state.MaxHP != 201 || state.Defeated {
		t.Fatalf("partial revive state=%#v", state)
	}
}

func TestRevivePercentRejectsInvalidPercentWithoutChangingState(t *testing.T) {
	service, err := NewService(200)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(3); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReduceHP(3, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevivePercent(3, 0); !errors.Is(err, ErrInvalidRevivePercent) {
		t.Fatalf("zero percent err=%v", err)
	}
	state, ok := service.State(3)
	if !ok || !state.Defeated || state.HP != 0 {
		t.Fatalf("state changed after invalid revive=%#v ok=%v", state, ok)
	}
}
