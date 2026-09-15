package equipmentaffix

import "testing"

type sequenceRoller struct {
	values []int
	index  int
}

func (r *sequenceRoller) Intn(n int) int {
	if r.index >= len(r.values) {
		return 0
	}
	value := r.values[r.index]
	r.index++
	return value
}

func TestAffixCount(t *testing.T) {
	cases := []struct {
		tier Tier
		want int
	}{
		{TierLow, 0},
		{TierMid, 1},
		{TierHigh, 2},
	}
	for _, tc := range cases {
		got, ok := AffixCount(tc.tier)
		if !ok || got != tc.want {
			t.Fatalf("AffixCount(%q) = (%d,%v), want (%d,true)", tc.tier, got, ok, tc.want)
		}
	}
}

func TestGenerateLowTierHasNoAffixes(t *testing.T) {
	affixes, err := Generate(TierLow, EquipmentKindWeapon, nil)
	if err != nil {
		t.Fatalf("Generate low: %v", err)
	}
	if len(affixes) != 0 {
		t.Fatalf("low affixes = %v, want none", affixes)
	}
}

func TestGenerateMidTierStrengthBoundaries(t *testing.T) {
	for _, tc := range []struct {
		roll int
		want uint8
	}{
		{0, 1},
		{8999, 1},
		{9000, 2},
		{9999, 2},
	} {
		r := &sequenceRoller{values: []int{0, tc.roll}}
		affixes, err := Generate(TierMid, EquipmentKindWeapon, r)
		if err != nil {
			t.Fatalf("roll %d: %v", tc.roll, err)
		}
		if len(affixes) != 1 || affixes[0].Strength != tc.want {
			t.Fatalf("roll %d => %#v, want strength %d", tc.roll, affixes, tc.want)
		}
	}
}

func TestGenerateHighTierStrengthBoundaries(t *testing.T) {
	for _, tc := range []struct {
		roll int
		want uint8
	}{
		{0, 1},
		{8999, 1},
		{9000, 2},
		{9699, 2},
		{9700, 3},
		{9949, 3},
		{9950, 4},
		{9994, 4},
		{9995, 5},
		{9999, 5},
	} {
		r := &sequenceRoller{values: []int{0, tc.roll, 0, 0}}
		affixes, err := Generate(TierHigh, EquipmentKindWeapon, r)
		if err != nil {
			t.Fatalf("roll %d: %v", tc.roll, err)
		}
		if len(affixes) != 2 {
			t.Fatalf("roll %d: len=%d, want 2", tc.roll, len(affixes))
		}
		found := false
		for _, affix := range affixes {
			if affix.Strength == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("roll %d => %#v, expected one strength %d", tc.roll, affixes, tc.want)
		}
	}
}

func TestGenerateHighTierNeverDuplicatesAffixID(t *testing.T) {
	// First selection uses index 0. Second selection again uses index 0, but the
	// first ID has already been removed from the available pool.
	r := &sequenceRoller{values: []int{0, 0, 0, 0}}
	affixes, err := Generate(TierHigh, EquipmentKindWeapon, r)
	if err != nil {
		t.Fatalf("Generate high: %v", err)
	}
	if len(affixes) != 2 {
		t.Fatalf("len=%d, want 2", len(affixes))
	}
	if affixes[0].ID == affixes[1].ID {
		t.Fatalf("duplicate affix IDs: %#v", affixes)
	}
}

func TestAllowedPoolsMatchFormalCounts(t *testing.T) {
	weapon, ok := AllowedPool(EquipmentKindWeapon)
	if !ok || len(weapon) != 10 {
		t.Fatalf("weapon pool = %d,%v, want 10,true", len(weapon), ok)
	}
	shield, ok := AllowedPool(EquipmentKindShield)
	if !ok || len(shield) != 11 {
		t.Fatalf("shield pool = %d,%v, want 11,true", len(shield), ok)
	}
}

func TestValueForFormalMappings(t *testing.T) {
	for strength := uint8(1); strength <= 5; strength++ {
		if got, ok := ValueFor(AffixStrength, strength); !ok || got != uint32(strength) {
			t.Fatalf("strength %d => %d,%v", strength, got, ok)
		}
		if got, ok := ValueFor(AffixMaxHP, strength); !ok || got != uint32(strength)*20 {
			t.Fatalf("max hp %d => %d,%v", strength, got, ok)
		}
		if got, ok := ValueFor(AffixMaxMP, strength); !ok || got != uint32(strength)*10 {
			t.Fatalf("max mp %d => %d,%v", strength, got, ok)
		}
	}
}

func TestValidateRejectsDuplicateAndWrongValue(t *testing.T) {
	duplicate := []Affix{
		{ID: AffixStrength, Strength: 1, Value: 1},
		{ID: AffixStrength, Strength: 2, Value: 2},
	}
	if err := Validate(TierHigh, EquipmentKindWeapon, duplicate); err == nil {
		t.Fatal("duplicate affixes unexpectedly valid")
	}
	wrongValue := []Affix{{ID: AffixMaxHP, Strength: 2, Value: 20}}
	if err := Validate(TierMid, EquipmentKindShield, wrongValue); err == nil {
		t.Fatal("wrong value unexpectedly valid")
	}
}

func TestValidateRejectsIllegalKindPool(t *testing.T) {
	affixes := []Affix{{ID: AffixMaxHP, Strength: 1, Value: 20}}
	if err := Validate(TierMid, EquipmentKindWeapon, affixes); err == nil {
		t.Fatal("weapon max-hp affix unexpectedly valid")
	}
}
