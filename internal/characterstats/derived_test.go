package characterstats

import (
	"errors"
	"math"
	"testing"
)

func TestMeleePhysicalDamageBonus(t *testing.T) {
	for _, tc := range []struct{ stat, want uint32 }{{9,0},{10,0},{11,0},{12,1},{19,4},{20,5}} {
		if got := MeleePhysicalDamageBonus(tc.stat); got != tc.want { t.Fatalf("strength=%d got=%d want=%d", tc.stat, got, tc.want) }
	}
}

func TestAgilityDerivedValues(t *testing.T) {
	for _, tc := range []struct{
		agility uint32
		hit int64
		ranged uint32
		evasion uint32
		critPP uint32
	}{
		{8,-2,0,0,0}, {10,0,0,0,0}, {12,2,1,1,0}, {18,8,3,4,0}, {40,30,10,15,1}, {50,40,13,20,2},
	} {
		if got := PhysicalHitModifier(tc.agility); got != tc.hit { t.Fatalf("agility=%d hit=%d want=%d", tc.agility, got, tc.hit) }
		if got := RangedPhysicalDamageBonus(tc.agility); got != tc.ranged { t.Fatalf("agility=%d ranged=%d want=%d", tc.agility, got, tc.ranged) }
		if got := EvasionModifier(tc.agility); got != tc.evasion { t.Fatalf("agility=%d evasion=%d want=%d", tc.agility, got, tc.evasion) }
		if got := AttributeCriticalBonusPercentPoints(tc.agility); got != tc.critPP { t.Fatalf("agility=%d crit=%d want=%d", tc.agility, got, tc.critPP) }
	}
}

func TestMagicPowerBonus(t *testing.T) {
	for _, tc := range []struct{ intelligence, want uint32 }{{10,0},{14,0},{15,1},{20,2},{34,4}} {
		if got := MagicPowerBonus(tc.intelligence); got != tc.want { t.Fatalf("int=%d got=%d want=%d", tc.intelligence, got, tc.want) }
	}
}

func TestBaseMaxVitalsGrowth(t *testing.T) {
	for _, tc := range []struct {
		level       uint32
		wantHP      uint64
		wantMP      uint64
	}{
		{1, 15, 6},
		{5, 59, 14},
		{10, 114, 24},
		{15, 169, 34},
		{18, 202, 40},
		{22, 246, 48},
		{50, 554, 104},
	} {
		hp, err := BaseMaxHP(tc.level)
		if err != nil {
			t.Fatalf("level=%d hp err=%v", tc.level, err)
		}
		mp, err := BaseMaxMP(tc.level)
		if err != nil {
			t.Fatalf("level=%d mp err=%v", tc.level, err)
		}
		if hp != tc.wantHP || mp != tc.wantMP {
			t.Fatalf("level=%d hp/mp=%d/%d want=%d/%d", tc.level, hp, mp, tc.wantHP, tc.wantMP)
		}
	}
	if _, err := BaseMaxHP(0); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("BaseMaxHP(0) err=%v", err)
	}
	if _, err := BaseMaxMP(0); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("BaseMaxMP(0) err=%v", err)
	}
}

func TestDerivedMaxVitalsComposesEffectiveAttributesAndFlatEquipment(t *testing.T) {
	maxHP, maxMP, err := DerivedMaxVitals(20, Primary{
		Strength: 10, Agility: 10, Constitution: 18,
		Intelligence: 10, Spirit: 16, Charisma: 10,
	}, 40, 20)
	if err != nil {
		t.Fatal(err)
	}
	if maxHP != 324 || maxMP != 104 {
		t.Fatalf("max hp/mp=%d/%d want=324/104", maxHP, maxMP)
	}

	neutralHP, neutralMP, err := DerivedMaxVitals(1, DefaultPrimary(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if neutralHP != 15 || neutralMP != 7 {
		t.Fatalf("neutral level-1 hp/mp=%d/%d want=15/7", neutralHP, neutralMP)
	}
}

func TestDerivedMaxVitalsRejectsUint32Overflow(t *testing.T) {
	if _, _, err := DerivedMaxVitals(math.MaxUint32, DefaultPrimary(), 0, 0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("overflow err=%v", err)
	}
}

func TestConstitutionMaxHPBonus(t *testing.T) {
	for _, tc := range []struct{ level, constitution uint32; want uint64 }{{20,15,0},{20,16,20},{20,18,60},{50,18,150},{50,20,250}} {
		if got := ConstitutionMaxHPBonus(tc.level, tc.constitution); got != tc.want { t.Fatalf("level=%d con=%d got=%d want=%d", tc.level, tc.constitution, got, tc.want) }
	}
}

func TestSpiritGrowthAndMaxMP(t *testing.T) {
	for _, tc := range []struct{ spirit, tier uint32 }{{9,0},{10,1},{14,1},{15,2},{20,2},{21,3},{24,3},{25,4},{28,4},{29,5},{32,5},{33,6}} {
		if got := SpiritGrowthTier(tc.spirit); got != tc.tier { t.Fatalf("spirit=%d tier=%d want=%d", tc.spirit, got, tc.tier) }
		if got := SpiritMaxMPBonus(20, tc.spirit); got != uint64(20*tc.tier) { t.Fatalf("spirit=%d mp=%d", tc.spirit, got) }
	}
}

func TestLeadershipCount(t *testing.T) {
	for _, tc := range []struct{ charisma, want uint32 }{{0,1},{10,1},{14,1},{15,2},{20,3},{25,4},{30,5},{100,5}} {
		if got := LeadershipCount(tc.charisma); got != tc.want { t.Fatalf("charisma=%d got=%d want=%d", tc.charisma, got, tc.want) }
	}
}

func TestPhysicalHitChanceBasisPoints(t *testing.T) {
	for _, tc := range []struct{ hit, evasion int64; want uint32 }{
		{0,0,9000}, {2,0,9100}, {0,2,8900}, {16,0,9800}, {100,0,9800}, {0,30,7500}, {0,100,7500},
	} {
		if got := PhysicalHitChanceBasisPoints(tc.hit, tc.evasion); got != tc.want { t.Fatalf("hit=%d evasion=%d got=%d want=%d", tc.hit, tc.evasion, got, tc.want) }
	}
}

func TestCriticalChanceBasisPoints(t *testing.T) {
	for _, tc := range []struct{ agility, rating, want uint32 }{
		{10,0,500}, {40,0,600}, {50,0,700}, {10,1,550}, {40,2,700}, {300,0,3000}, {10,50,3000},
	} {
		if got := CriticalChanceBasisPoints(tc.agility, tc.rating); got != tc.want { t.Fatalf("agi=%d rating=%d got=%d want=%d", tc.agility, tc.rating, got, tc.want) }
	}
}