package equipmentaffix

import "testing"

func TestArmorAffixPoolMatchesFormalPlan(t *testing.T) {
	pool, ok := AllowedPool(EquipmentKindArmor)
	if !ok {
		t.Fatal("armor pool missing")
	}
	allowed := make(map[AffixID]bool, len(pool))
	for _, id := range pool {
		allowed[id] = true
	}
	for _, id := range []AffixID{
		AffixStrength, AffixDexterity, AffixIntelligence, AffixConstitution, AffixSpirit, AffixCharisma,
		AffixPhysicalHit, AffixCriticalRating, AffixEvasion, AffixMaxHP, AffixMaxMP,
		AffixPhysicalDefense, AffixMagicDefense, AffixMagicPower,
	} {
		if !allowed[id] {
			t.Fatalf("armor affix %q missing", id)
		}
	}
	if allowed[AffixPhysicalDamage] {
		t.Fatal("armor pool must not include physical-damage affix")
	}
}

type armorRoller struct {
	values []int
	index  int
}

func (r *armorRoller) Intn(n int) int {
	if len(r.values) == 0 {
		return 0
	}
	value := r.values[r.index%len(r.values)]
	r.index++
	if n <= 0 {
		return value
	}
	return value % n
}

func TestHighArmorRollsTwoDifferentAffixes(t *testing.T) {
	roller := &armorRoller{values: []int{0, 0, 0, 0}}
	affixes, err := Generate(TierHigh, EquipmentKindArmor, roller)
	if err != nil {
		t.Fatal(err)
	}
	if len(affixes) != 2 {
		t.Fatalf("affix count=%d want=2", len(affixes))
	}
	if affixes[0].ID == affixes[1].ID {
		t.Fatalf("duplicate high armor affix=%#v", affixes)
	}
	if err := Validate(TierHigh, EquipmentKindArmor, affixes); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
}
