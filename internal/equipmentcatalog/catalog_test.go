package equipmentcatalog

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
)

func TestDefaultCatalogLocksLowTierEquipmentV1(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "low-tier-equipment-v1" {
		t.Fatalf("revision = %q", got)
	}

	cases := []struct {
		id       string
		small    DamageRange
		large    DamageRange
		extra    uint32
		accuracy int32
		interval uint32
		weight   uint32
	}{
		{"item_militia_iron_sword", DamageRange{6, 9}, DamageRange{6, 8}, 0, 0, 1000, 7},
		{"item_light_guard_sword", DamageRange{5, 8}, DamageRange{5, 7}, 0, 2, 850, 6},
		{"item_gladiator_iron_sword", DamageRange{7, 10}, DamageRange{6, 9}, 0, 0, 1100, 8},
		{"item_militia_battle_axe", DamageRange{5, 8}, DamageRange{8, 12}, 0, -1, 1150, 10},
		{"item_iron_war_mace", DamageRange{6, 9}, DamageRange{6, 9}, 1, 1, 1150, 9},
	}
	for _, tc := range cases {
		item, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing %s", tc.id)
		}
		if item.Kind != KindWeapon || item.Slot != SlotMainHand || item.Weapon == nil {
			t.Fatalf("%s not main-hand weapon: %#v", tc.id, item)
		}
		if item.Weapon.SmallDamage != tc.small || item.Weapon.LargeDamage != tc.large || item.Weapon.ExtraDamage != tc.extra || item.Weapon.AccuracyModifier != tc.accuracy || item.Weapon.BasicAttackIntervalMS != tc.interval || item.Weight != tc.weight {
			t.Fatalf("%s unexpected values: %#v", tc.id, item)
		}
		if item.ClassPolicy != ClassPolicyAll || !item.AllowsClass("") || len(item.AllowedClassIDs) != 0 {
			t.Fatalf("%s class policy = %#v", tc.id, item)
		}
	}

	shields := []struct {
		id                                    string
		physical                              uint32
		block, blockReduction, magicReduction uint8
		weight                                uint32
	}{
		{"item_iron_rim_round_shield", 3, 8, 25, 0, 6},
		{"item_guard_shield", 4, 10, 30, 0, 8},
		{"item_runed_square_shield", 2, 6, 20, 8, 6},
	}
	for _, tc := range shields {
		item, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing %s", tc.id)
		}
		if item.Kind != KindShield || item.Slot != SlotOffHand || item.Shield == nil {
			t.Fatalf("%s not off-hand shield: %#v", tc.id, item)
		}
		if item.Shield.PhysicalDefense != tc.physical || item.Shield.BlockChancePercent != tc.block || item.Shield.BlockDamageReductionPercent != tc.blockReduction || item.Weight != tc.weight {
			t.Fatalf("%s unexpected values: %#v", tc.id, item)
		}
		if item.ClassPolicy != ClassPolicyAll || !item.AllowsClass("future-class") {
			t.Fatalf("%s should be usable by all classes: %#v", tc.id, item)
		}
	}
}

func TestClassPolicyAllowListIsCanonicalAndFailClosed(t *testing.T) {
	def := Definition{
		ItemArchetypeID: "item_test_shield",
		Kind:            KindShield,
		Slot:            SlotOffHand,
		Weight:          1,
		Material:        "iron",
		ClassPolicy:     ClassPolicyAllowList,
		AllowedClassIDs: []string{string(classid.Oathguard), string(classid.Breaker)},
		Shield:          &Shield{PhysicalDefense: 1},
	}
	catalog, err := New(CatalogDefinition{Revision: "x", Items: []Definition{def}})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := catalog.Resolve(def.ItemArchetypeID)
	if !ok {
		t.Fatal("missing item")
	}
	if !item.AllowsClass(string(classid.Oathguard)) || !item.AllowsClass(string(classid.Breaker)) {
		t.Fatalf("canonical classes rejected: %#v", item)
	}
	for _, classID := range []string{"", "class_guard", "class_knight", string(classid.Ranger), " class_oathguard"} {
		if item.AllowsClass(classID) {
			t.Fatalf("AllowsClass(%q) unexpectedly true", classID)
		}
	}
}

func TestCatalogRejectsMalformedDefinitions(t *testing.T) {
	validWeapon := Definition{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAll, Weapon: &Weapon{SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}, BasicAttackIntervalMS: 1000}}
	for name, def := range map[string]CatalogDefinition{
		"duplicate":          {Revision: "x", Items: []Definition{validWeapon, validWeapon}},
		"weapon_in_offhand":  {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotOffHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAll, Weapon: validWeapon.Weapon}}},
		"invalid_range":      {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAll, Weapon: &Weapon{SmallDamage: DamageRange{3, 2}, LargeDamage: DamageRange{1, 2}, BasicAttackIntervalMS: 1000}}}},
		"empty_allow_list":   {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAllowList, Weapon: validWeapon.Weapon}}},
		"duplicate_class_id": {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAllowList, AllowedClassIDs: []string{string(classid.Oathguard), string(classid.Oathguard)}, Weapon: validWeapon.Weapon}}},
		"unknown_class_id":   {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAllowList, AllowedClassIDs: []string{"class_guard"}, Weapon: validWeapon.Weapon}}},
		"padded_class_id":    {Revision: "x", Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", ClassPolicy: ClassPolicyAllowList, AllowedClassIDs: []string{" class_oathguard"}, Weapon: validWeapon.Weapon}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(def); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
