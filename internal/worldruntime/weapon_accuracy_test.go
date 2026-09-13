package worldruntime

import (
	"math"
	"testing"
)

func TestWeaponBasicAttackHitChanceUsesFormalRatingScale(t *testing.T) {
	for _, tc := range []struct {
		name     string
		modifier int32
		want     uint32
	}{
		{name: "militia sword", modifier: 0, want: 9000},
		{name: "light guard sword", modifier: 2, want: 9100},
		{name: "battle axe", modifier: -1, want: 8950},
		{name: "war mace", modifier: 1, want: 9050},
		{name: "minimum clamp", modifier: math.MinInt32, want: 7500},
		{name: "maximum clamp", modifier: math.MaxInt32, want: 9800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := weaponBasicAttackHitChanceBasisPoints(tc.modifier, 0, 0); got != tc.want {
				t.Fatalf("hit chance = %d basis points, want %d", got, tc.want)
			}
		})
	}
}

func TestWeaponBasicAttackHitAndEvasionRatingsOffsetEachOther(t *testing.T) {
	if got := weaponBasicAttackHitChanceBasisPoints(0, 5, 5); got != 9000 {
		t.Fatalf("equal hit/evasion ratings => %d, want 9000", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(0, 5, 0); got != 9250 {
		t.Fatalf("+5 hit rating => %d, want 9250", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(0, 0, 5); got != 8750 {
		t.Fatalf("+5 evasion rating => %d, want 8750", got)
	}
}

func TestWeaponBasicAttackHitRollBoundary(t *testing.T) {
	const modifier int32 = -1 // 89.5%
	if !weaponBasicAttackHits(modifier, 0, 0, 8949) {
		t.Fatal("roll 8949 should hit at 89.5%")
	}
	if weaponBasicAttackHits(modifier, 0, 0, 8950) {
		t.Fatal("roll 8950 should miss at 89.5%")
	}
	if weaponBasicAttackHits(modifier, 0, 0, 9999) {
		t.Fatal("roll 9999 should miss at 89.5%")
	}
}
