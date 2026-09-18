package combat

import "testing"

func TestPhysicalHitChanceBasisPoints(t *testing.T) {
	cases := []struct {
		hit, evasion int32
		want         uint32
	}{
		{0, 0, 9000},
		{5, 0, 9250},
		{0, 5, 8750},
		{5, 5, 9000},
		{100, 0, 9800},
		{-100, 0, 7500},
	}
	for _, tc := range cases {
		if got := PhysicalHitChanceBasisPoints(tc.hit, tc.evasion); got != tc.want {
			t.Fatalf("hit=%d evasion=%d => %d, want %d", tc.hit, tc.evasion, got, tc.want)
		}
	}
}

func TestCriticalChanceBasisPoints(t *testing.T) {
	cases := []struct {
		attribute uint32
		rating    uint32
		want      uint32
	}{
		{0, 0, 500},
		{0, 5, 750},
		{100, 5, 850},
		{5000, 100, 3000},
	}
	for _, tc := range cases {
		if got := CriticalChanceBasisPoints(tc.attribute, tc.rating); got != tc.want {
			t.Fatalf("attribute=%d rating=%d => %d, want %d", tc.attribute, tc.rating, got, tc.want)
		}
	}
}

func TestDefenseMitigationBasisPoints(t *testing.T) {
	cases := []struct {
		defense uint32
		want    uint32
	}{
		{0, 0},
		{5, 476},
		{20, 1666},
		{60, 3750},
	}
	for _, tc := range cases {
		if got := DefenseMitigationBasisPoints(tc.defense); got != tc.want {
			t.Fatalf("defense=%d => %d, want %d", tc.defense, got, tc.want)
		}
	}
}

func TestMagicDefenseThenShieldReductionUsesSeparateMultipliers(t *testing.T) {
	magicDefenseRemaining := RemainingBasisPointsAfterMitigation(DefenseMitigationBasisPoints(5))
	if magicDefenseRemaining != 9524 {
		t.Fatalf("magic defense remaining = %d, want 9524", magicDefenseRemaining)
	}
	shieldRemaining := uint32(9200)
	combined := CombineRemainingBasisPoints(magicDefenseRemaining, shieldRemaining)
	if combined != 8762 {
		t.Fatalf("combined remaining = %d, want 8762", combined)
	}
}

func TestCriticalMultiplierV1IsThreeHalves(t *testing.T) {
	if CriticalMultiplierV1Numerator != 3 || CriticalMultiplierV1Denominator != 2 {
		t.Fatalf("critical multiplier = %d/%d, want 3/2", CriticalMultiplierV1Numerator, CriticalMultiplierV1Denominator)
	}
}
