package characterstats

import (
	"errors"
	"math"
	"testing"
)

func TestEffectiveAppliesAuthoredAdditiveBonuses(t *testing.T) {
	got, err := Effective(
		Primary{Strength: 10, Agility: 20},
		AdditiveBonus{Strength: 5, Agility: 5},
	)
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}
	want := Primary{Strength: 15, Agility: 25}
	if got != want {
		t.Fatalf("Effective() = %+v, want %+v", got, want)
	}
}

func TestEffectiveKeepsUnmodifiedAttributesStable(t *testing.T) {
	base := Primary{Strength: 7, Agility: 11}
	got, err := Effective(base, AdditiveBonus{})
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}
	if got != base {
		t.Fatalf("Effective() = %+v, want %+v", got, base)
	}
}

func TestEffectiveRejectsOverflow(t *testing.T) {
	_, err := Effective(
		Primary{Strength: math.MaxUint32, Agility: 1},
		AdditiveBonus{Strength: 1},
	)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Effective() error = %v, want %v", err, ErrOverflow)
	}
}
