package iteminstance

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

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

func testWeaponDefinition(tier equipmentcatalog.Tier) equipmentcatalog.Definition {
	return equipmentcatalog.Definition{
		ItemArchetypeID: "item_test_weapon",
		Kind:            equipmentcatalog.KindWeapon,
		Slot:            equipmentcatalog.SlotMainHand,
		Tier:            tier,
		Weight:          1,
		Material:        "iron",
		Weapon: &equipmentcatalog.Weapon{
			WeaponType:  equipmentcatalog.WeaponTypeOneHandSword,
			SmallDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1},
			LargeDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1},
		},
	}
}

func TestCreateLowTierInstanceHasNoAffixes(t *testing.T) {
	instance, err := Create("instance-low-1", testWeaponDefinition(equipmentcatalog.TierLow), nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(instance.Affixes) != 0 {
		t.Fatalf("low affixes = %#v, want none", instance.Affixes)
	}
}

func TestCreateHighTierInstancePersistsTwoDistinctAffixes(t *testing.T) {
	definition := testWeaponDefinition(equipmentcatalog.TierHigh)
	r := &sequenceRoller{values: []int{0, 9995, 0, 9700}}
	instance, err := Create("instance-high-1", definition, r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(instance.Affixes) != 2 {
		t.Fatalf("affix count=%d, want 2", len(instance.Affixes))
	}
	if instance.Affixes[0].ID == instance.Affixes[1].ID {
		t.Fatalf("duplicate affixes: %#v", instance.Affixes)
	}
	data, err := CanonicalJSON(instance, definition)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	restored, err := DecodeCanonicalJSON(data, definition)
	if err != nil {
		t.Fatalf("DecodeCanonicalJSON: %v", err)
	}
	if len(restored.Affixes) != 2 {
		t.Fatalf("restored affix count=%d, want 2", len(restored.Affixes))
	}
	for i := range instance.Affixes {
		if restored.Affixes[i] != instance.Affixes[i] {
			t.Fatalf("restored[%d]=%#v, want %#v", i, restored.Affixes[i], instance.Affixes[i])
		}
	}
}

func TestDecodeCannotRerollOrAcceptChangedValue(t *testing.T) {
	definition := testWeaponDefinition(equipmentcatalog.TierMid)
	instance := Instance{
		ID:              "instance-mid-1",
		ItemArchetypeID: definition.ItemArchetypeID,
		Affixes: []equipmentaffix.Affix{{
			ID:       equipmentaffix.AffixStrength,
			Strength: 1,
			Value:    1,
		}},
	}
	data, err := CanonicalJSON(instance, definition)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	data[len(data)-3] = '2'
	if _, err := DecodeCanonicalJSON(data, definition); err == nil {
		t.Fatal("tampered instance unexpectedly accepted")
	}
}

func TestValidateRejectsArchetypeMismatch(t *testing.T) {
	definition := testWeaponDefinition(equipmentcatalog.TierLow)
	instance := Instance{ID: "instance-low-1", ItemArchetypeID: "other"}
	if err := Validate(instance, definition); err == nil {
		t.Fatal("archetype mismatch unexpectedly valid")
	}
}
