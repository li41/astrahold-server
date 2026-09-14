package characterstats

import (
	"errors"
	"math"
	"testing"
)

func TestDefaultPrimaryIsFormalNeutralBaseline(t *testing.T) {
	want := Primary{Strength: 10, Agility: 10, Constitution: 10, Intelligence: 10, Spirit: 10, Charisma: 10}
	if got := DefaultPrimary(); got != want {
		t.Fatalf("DefaultPrimary() = %+v, want %+v", got, want)
	}
	if err := ValidateBase(want); err != nil {
		t.Fatalf("ValidateBase(default) error = %v", err)
	}
}

func TestValidateBaseRejectsAnyAttributeBelowTen(t *testing.T) {
	base := DefaultPrimary()
	tests := []Primary{
		func() Primary { v := base; v.Strength = 9; return v }(),
		func() Primary { v := base; v.Agility = 9; return v }(),
		func() Primary { v := base; v.Constitution = 9; return v }(),
		func() Primary { v := base; v.Intelligence = 9; return v }(),
		func() Primary { v := base; v.Spirit = 9; return v }(),
		func() Primary { v := base; v.Charisma = 9; return v }(),
	}
	for i, primary := range tests {
		if err := ValidateBase(primary); !errors.Is(err, ErrInvalidBase) {
			t.Fatalf("case %d error = %v, want %v", i, err, ErrInvalidBase)
		}
	}
}

func TestEffectiveAppliesAuthoredAdditiveBonuses(t *testing.T) {
	got, err := Effective(
		Primary{Strength: 10, Agility: 20, Constitution: 11, Intelligence: 12, Spirit: 13, Charisma: 14},
		AdditiveBonus{Strength: 5, Agility: 5, Constitution: 1, Intelligence: 2, Spirit: 3, Charisma: 4},
	)
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}
	want := Primary{Strength: 15, Agility: 25, Constitution: 12, Intelligence: 14, Spirit: 16, Charisma: 18}
	if got != want {
		t.Fatalf("Effective() = %+v, want %+v", got, want)
	}
}

func TestEffectiveKeepsUnmodifiedAttributesStable(t *testing.T) {
	base := Primary{Strength: 10, Agility: 11, Constitution: 12, Intelligence: 13, Spirit: 14, Charisma: 15}
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
		Primary{Strength: math.MaxUint32, Agility: 10, Constitution: 10, Intelligence: 10, Spirit: 10, Charisma: 10},
		AdditiveBonus{Strength: 1},
	)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Effective() error = %v, want %v", err, ErrOverflow)
	}
}