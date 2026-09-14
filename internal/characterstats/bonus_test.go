package characterstats

import (
	"errors"
	"math"
	"testing"
)

func TestCombineAdditive(t *testing.T) {
	got, err := CombineAdditive(AdditiveBonus{Strength: 5, Agility: 2}, AdditiveBonus{Strength: 3, Agility: 7})
	if err != nil { t.Fatal(err) }
	want := AdditiveBonus{Strength: 8, Agility: 9}
	if got != want { t.Fatalf("got=%#v want=%#v", got, want) }
}

func TestCombineAdditiveRejectsOverflow(t *testing.T) {
	if _, err := CombineAdditive(AdditiveBonus{Strength: math.MaxUint32}, AdditiveBonus{Strength: 1}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("err=%v", err)
	}
	if _, err := CombineAdditive(AdditiveBonus{Agility: math.MaxUint32}, AdditiveBonus{Agility: 1}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("err=%v", err)
	}
}