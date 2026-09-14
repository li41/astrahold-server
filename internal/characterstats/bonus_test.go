package characterstats

import (
	"errors"
	"math"
	"testing"
)

func TestCombineAdditive(t *testing.T) {
	got, err := CombineAdditive(
		AdditiveBonus{Strength: 5, Agility: 2, Constitution: 1, Intelligence: 4, Spirit: 3, Charisma: 6},
		AdditiveBonus{Strength: 3, Agility: 7, Constitution: 8, Intelligence: 2, Spirit: 5, Charisma: 1},
	)
	if err != nil { t.Fatal(err) }
	want := AdditiveBonus{Strength: 8, Agility: 9, Constitution: 9, Intelligence: 6, Spirit: 8, Charisma: 7}
	if got != want { t.Fatalf("got=%#v want=%#v", got, want) }
}

func TestCombineAdditiveRejectsOverflow(t *testing.T) {
	cases := []struct{
		left AdditiveBonus
		right AdditiveBonus
	}{
		{left: AdditiveBonus{Strength: math.MaxUint32}, right: AdditiveBonus{Strength: 1}},
		{left: AdditiveBonus{Agility: math.MaxUint32}, right: AdditiveBonus{Agility: 1}},
		{left: AdditiveBonus{Constitution: math.MaxUint32}, right: AdditiveBonus{Constitution: 1}},
		{left: AdditiveBonus{Intelligence: math.MaxUint32}, right: AdditiveBonus{Intelligence: 1}},
		{left: AdditiveBonus{Spirit: math.MaxUint32}, right: AdditiveBonus{Spirit: 1}},
		{left: AdditiveBonus{Charisma: math.MaxUint32}, right: AdditiveBonus{Charisma: 1}},
	}
	for i, tc := range cases {
		if _, err := CombineAdditive(tc.left, tc.right); !errors.Is(err, ErrOverflow) {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
}