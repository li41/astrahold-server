package worldruntime

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
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
			if got := weaponBasicAttackHitChanceBasisPoints(tc.modifier, 0, 0, 0, 0); got != tc.want {
				t.Fatalf("hit chance = %d basis points, want %d", got, tc.want)
			}
		})
	}
}

func TestWeaponBasicAttackHitAndEvasionRatingsOffsetEachOther(t *testing.T) {
	if got := weaponBasicAttackHitChanceBasisPoints(0, 0, 5, 0, 5); got != 9000 {
		t.Fatalf("equal equipment hit/evasion ratings => %d, want 9000", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(0, 0, 5, 0, 0); got != 9250 {
		t.Fatalf("+5 equipment hit rating => %d, want 9250", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(0, 0, 0, 0, 5); got != 8750 {
		t.Fatalf("+5 equipment evasion rating => %d, want 8750", got)
	}
}

func TestWeaponBasicAttackAgilityRatingsComposeWithEquipment(t *testing.T) {
	attackerHit := characterstats.PhysicalHitModifier(14) // +4 rating.
	targetEvasion := characterstats.EvasionModifier(14)  // +2 rating.

	if got := weaponBasicAttackHitChanceBasisPoints(0, attackerHit, 0, 0, 0); got != 9200 {
		t.Fatalf("AGI 14 attacker hit chance = %d, want 9200", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(0, 0, 0, targetEvasion, 0); got != 8900 {
		t.Fatalf("AGI 14 target evasion chance = %d, want 8900", got)
	}
	if got := weaponBasicAttackHitChanceBasisPoints(2, attackerHit, 3, targetEvasion, 1); got != 9300 {
		t.Fatalf("combined weapon/attribute/equipment chance = %d, want 9300", got)
	}
}

func TestWeaponBasicAttackHitRollBoundary(t *testing.T) {
	const modifier int32 = -1 // 89.5%
	if !weaponBasicAttackHits(modifier, 0, 0, 0, 0, 8949) {
		t.Fatal("roll 8949 should hit at 89.5%")
	}
	if weaponBasicAttackHits(modifier, 0, 0, 0, 0, 8950) {
		t.Fatal("roll 8950 should miss at 89.5%")
	}
	if weaponBasicAttackHits(modifier, 0, 0, 0, 0, 9999) {
		t.Fatal("roll 9999 should miss at 89.5%")
	}
}
