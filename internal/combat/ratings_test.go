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
		{5, 2000},
		{20, 5000},
		{60, 7500},
	}
	for _, tc := range cases {
		if got := DefenseMitigationBasisPoints(tc.defense); got != tc.want {
			t.Fatalf("defense=%d => %d, want %d", tc.defense, got, tc.want)
		}
	}
}

func TestMagicDefenseThenShieldReductionUsesSeparateMultipliers(t *testing.T) {
	amount := uint32(100)
	afterDefense := ApplyMitigationBasisPoints(amount, DefenseMitigationBasisPoints(5))
	if afterDefense != 80 {
		t.Fatalf("after defense = %d, want 80", afterDefense)
	}
	afterShield := ApplyMitigationBasisPoints(afterDefense, 800)
	if afterShield != 73 {
		t.Fatalf("after shield = %d, want integer-stage result 73", afterShield)
	}
}

func TestApplyCriticalMultiplierV1(t *testing.T) {
	if got := ApplyCriticalMultiplierV1(20); got != 30 {
		t.Fatalf("20 critical => %d, want 30", got)
	}
	if got := ApplyCriticalMultiplierV1(21); got != 31 {
		t.Fatalf("21 critical => %d, want 31 with integer truncation", got)
	}
}
