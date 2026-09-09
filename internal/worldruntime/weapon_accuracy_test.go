package worldruntime

import (
	"math"
	"testing"
)

func TestWeaponBasicAttackHitChanceV1(t *testing.T) {
	for _, tc := range []struct {
		name     string
		modifier int32
		want     uint32
	}{
		{name: "militia sword", modifier: 0, want: 90},
		{name: "light guard sword", modifier: 2, want: 94},
		{name: "battle axe", modifier: -1, want: 88},
		{name: "war mace", modifier: 1, want: 92},
		{name: "minimum clamp", modifier: math.MinInt32, want: 75},
		{name: "maximum clamp", modifier: math.MaxInt32, want: 98},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := weaponBasicAttackHitChancePercent(tc.modifier); got != tc.want {
				t.Fatalf("hit chance = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWeaponBasicAttackHitRollBoundary(t *testing.T) {
	const modifier int32 = -1 // 88%
	if !weaponBasicAttackHits(modifier, 87) {
		t.Fatal("roll 87 should hit at 88%")
	}
	if weaponBasicAttackHits(modifier, 88) {
		t.Fatal("roll 88 should miss at 88%")
	}
	if weaponBasicAttackHits(modifier, 99) {
		t.Fatal("roll 99 should miss at 88%")
	}
}
