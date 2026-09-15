package equipmentcatalog

import (
	"errors"
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
)

func TestDefaultCatalogLocksEquipmentProgressionV1(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "weapon-progression-v1" {
		t.Fatalf("revision = %q", got)
	}

	cases := []struct {
		id              string
		weaponType      WeaponType
		small           DamageRange
		large           DamageRange
		extra           uint32
		accuracy        int32
		weight          uint32
		intervalMS      uint32
		damageAttribute characterstats.ID
	}{
		{"item_militia_iron_sword", WeaponTypeOneHandSword, DamageRange{6, 9}, DamageRange{6, 8}, 0, 0, 7, 900, characterstats.Strength},
		{"item_light_guard_sword", WeaponTypeOneHandSword, DamageRange{5, 8}, DamageRange{5, 7}, 0, 2, 6, 900, characterstats.Strength},
		{"item_gladiator_iron_sword", WeaponTypeOneHandSword, DamageRange{7, 10}, DamageRange{6, 9}, 0, 0, 8, 900, characterstats.Strength},
		{"item_militia_battle_axe", WeaponTypeOneHandAxe, DamageRange{5, 8}, DamageRange{8, 12}, 0, -1, 10, 1050, characterstats.Strength},
		{"item_iron_war_mace", WeaponTypeMace, DamageRange{6, 9}, DamageRange{6, 9}, 1, 1, 9, 1050, characterstats.Strength},
	}
	for _, tc := range cases {
		item, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing %s", tc.id)
		}
		if item.Kind != KindWeapon || item.Slot != SlotMainHand || item.Tier != TierLow || item.Weapon == nil {
			t.Fatalf("%s not low-tier main-hand weapon: %#v", tc.id, item)
		}
		if item.Weapon.WeaponType != tc.weaponType || item.Weapon.SmallDamage != tc.small || item.Weapon.LargeDamage != tc.large || item.Weapon.ExtraDamage != tc.extra || item.Weapon.AccuracyModifier != tc.accuracy || item.Weight != tc.weight {
			t.Fatalf("%s unexpected values: %#v", tc.id, item)
		}
		interval, authored := catalog.BasicAttackIntervalMSForItem(tc.id)
		if !authored || interval != tc.intervalMS {
			t.Fatalf("%s cadence = %d authored=%v, want %d", tc.id, interval, authored, tc.intervalMS)
		}
		attribute, authored := catalog.BasicAttackDamageAttributeForItem(tc.id)
		if !authored || attribute != tc.damageAttribute {
			t.Fatalf("%s damage attribute = %q authored=%v, want %q", tc.id, attribute, authored, tc.damageAttribute)
		}
		if attackRange, authored := catalog.BasicAttackRangeForItem(tc.id); authored || attackRange != 0 {
			t.Fatalf("melee weapon %s unexpectedly has range override %v", tc.id, attackRange)
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
		if item.Kind != KindShield || item.Slot != SlotOffHand || item.Tier != TierLow || item.Shield == nil {
			t.Fatalf("%s not low-tier off-hand shield: %#v", tc.id, item)
		}
		if item.Shield.PhysicalDefense != tc.physical || item.Shield.BlockChancePercent != tc.block || item.Shield.BlockDamageReductionPercent != tc.blockReduction || item.Shield.MagicDamageReductionPercent != tc.magicReduction || item.Weight != tc.weight {
			t.Fatalf("%s unexpected values: %#v", tc.id, item)
		}
		if attribute, authored := catalog.BasicAttackDamageAttributeForItem(tc.id); authored || attribute != "" {
			t.Fatalf("shield %s unexpectedly has basic-attack damage attribute %q", tc.id, attribute)
		}
		if attackRange, authored := catalog.BasicAttackRangeForItem(tc.id); authored || attackRange != 0 {
			t.Fatalf("shield %s unexpectedly has basic-attack range %v", tc.id, attackRange)
		}
	}
}

func TestDefaultCatalogLocksFormalWeaponTypeCadenceScalingAndRange(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[WeaponType]uint32{
		"one_hand_sword": 900,
		"dagger": 650,
		"one_hand_axe": 1050,
		"one_hand_spear": 1000,
		"warhammer": 1150,
		"morning_star": 1200,
		"mace": 1050,
		"two_hand_sword": 1350,
		"two_hand_axe": 1500,
		"two_hand_spear": 1300,
		"knuckles": 600,
		"claw": 700,
		"dual_blades": 700,
		"bow": 1200,
		"crossbow": 1550,
		"sling": 1100,
		"staff": 1250,
	}
	wantDamageAttribute := map[WeaponType]characterstats.ID{
		"one_hand_sword": characterstats.Strength,
		"dagger":         characterstats.Strength,
		"one_hand_axe":   characterstats.Strength,
		"one_hand_spear": characterstats.Strength,
		"warhammer":      characterstats.Strength,
		"morning_star":   characterstats.Strength,
		"mace":           characterstats.Strength,
		"two_hand_sword": characterstats.Strength,
		"two_hand_axe":   characterstats.Strength,
		"two_hand_spear": characterstats.Strength,
		"knuckles":       characterstats.Strength,
		"claw":           characterstats.Strength,
		"dual_blades":    characterstats.Strength,
		"bow":            characterstats.Agility,
		"crossbow":       characterstats.Agility,
		"sling":          characterstats.Agility,
	}
	wantRange := map[WeaponType]float32{
		"sling":    14,
		"bow":      18,
		"crossbow": 22,
	}
	if len(catalog.weaponTypes) != len(want) {
		t.Fatalf("weapon type count = %d, want %d", len(catalog.weaponTypes), len(want))
	}
	for weaponType, intervalMS := range want {
		definition, ok := catalog.weaponTypes[weaponType]
		if !ok || definition.BasicAttackIntervalMS == nil || *definition.BasicAttackIntervalMS != intervalMS {
			t.Fatalf("%s cadence = %#v, want %dms", weaponType, definition.BasicAttackIntervalMS, intervalMS)
		}
		if definition.BasicAttackDamageAttribute != wantDamageAttribute[weaponType] {
			t.Fatalf("%s damage attribute = %q, want %q", weaponType, definition.BasicAttackDamageAttribute, wantDamageAttribute[weaponType])
		}
		rangeValue, hasRange := wantRange[weaponType]
		if !hasRange {
			if definition.BasicAttackRange != nil {
				t.Fatalf("%s range = %v, want action fallback", weaponType, *definition.BasicAttackRange)
			}
			continue
		}
		if definition.BasicAttackRange == nil || *definition.BasicAttackRange != rangeValue {
			t.Fatalf("%s range = %#v, want %v", weaponType, definition.BasicAttackRange, rangeValue)
		}
	}
}

func TestCatalogWeaponTypeOwnsSharedBasicAttackInterval(t *testing.T) {
	interval := uint32(975)
	catalog, err := New(CatalogDefinition{
		Revision: "shared-cadence-test",
		WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackIntervalMS: &interval}},
		Items: []Definition{
			{ItemArchetypeID: "item_sword_a", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}}},
			{ItemArchetypeID: "item_sword_b", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 2, Material: "steel", Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{2, 3}, LargeDamage: DamageRange{2, 3}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, itemID := range []string{"item_sword_a", "item_sword_b"} {
		got, ok := catalog.BasicAttackIntervalMSForItem(itemID)
		if !ok || got != interval {
			t.Fatalf("%s cadence = %d ok=%v, want %d", itemID, got, ok, interval)
		}
	}
}

func TestCatalogWeaponTypeOwnsSharedBasicAttackDamageAttribute(t *testing.T) {
	bow := WeaponType("bow")
	catalog, err := New(CatalogDefinition{
		Revision: "shared-damage-attribute-test",
		WeaponTypes: []WeaponTypeDefinition{{
			WeaponType: bow, BasicAttackDamageAttribute: characterstats.Agility,
		}},
		Items: []Definition{
			{ItemArchetypeID: "item_bow_a", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "wood", Weapon: &Weapon{WeaponType: bow, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}}},
			{ItemArchetypeID: "item_bow_b", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 2, Material: "wood", Weapon: &Weapon{WeaponType: bow, SmallDamage: DamageRange{2, 3}, LargeDamage: DamageRange{2, 3}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, itemID := range []string{"item_bow_a", "item_bow_b"} {
		got, ok := catalog.BasicAttackDamageAttributeForItem(itemID)
		if !ok || got != characterstats.Agility {
			t.Fatalf("%s damage attribute = %q ok=%v, want %q", itemID, got, ok, characterstats.Agility)
		}
	}
}

func TestCatalogWeaponTypeOwnsSharedBasicAttackRange(t *testing.T) {
	bow := WeaponType("bow")
	attackRange := float32(18)
	catalog, err := New(CatalogDefinition{
		Revision: "shared-range-test",
		WeaponTypes: []WeaponTypeDefinition{{
			WeaponType: bow, BasicAttackRange: &attackRange,
		}},
		Items: []Definition{
			{ItemArchetypeID: "item_bow_a", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "wood", Weapon: &Weapon{WeaponType: bow, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}}},
			{ItemArchetypeID: "item_bow_b", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierHigh, Weight: 2, Material: "wood", Weapon: &Weapon{WeaponType: bow, SmallDamage: DamageRange{2, 3}, LargeDamage: DamageRange{2, 3}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, itemID := range []string{"item_bow_a", "item_bow_b"} {
		got, ok := catalog.BasicAttackRangeForItem(itemID)
		if !ok || got != attackRange {
			t.Fatalf("%s range = %v ok=%v, want %v", itemID, got, ok, attackRange)
		}
	}
}

func TestCatalogRejectsPerItemBasicAttackInterval(t *testing.T) {
	data := []byte(`{
		"revision":"x",
		"weapon_types":[{"weapon_type":"one_hand_sword"}],
		"items":[{
			"item_archetype_id":"item_test",
			"kind":"weapon",
			"slot":"main_hand",
			"tier":"low",
			"weight":1,
			"material":"iron",
			"weapon":{
				"weapon_type":"one_hand_sword",
				"small_damage":{"min":1,"max":2},
				"large_damage":{"min":1,"max":2},
				"basic_attack_interval_ms":1000
			}
		}]
	}`)
	if _, err := Load(data); err == nil {
		t.Fatal("per-item basic_attack_interval_ms unexpectedly accepted")
	}
}

func TestCatalogRejectsLegacyClassPolicyFields(t *testing.T) {
	data := []byte(`{
		"revision":"x",
		"weapon_types":[{"weapon_type":"one_hand_sword"}],
		"items":[{
			"item_archetype_id":"item_test",
			"kind":"weapon",
			"slot":"main_hand",
			"tier":"low",
			"weight":1,
			"material":"iron",
			"class_policy":"all",
			"weapon":{
				"weapon_type":"one_hand_sword",
				"small_damage":{"min":1,"max":2},
				"large_damage":{"min":1,"max":2}
			}
		}]
	}`)
	if _, err := Load(data); err == nil {
		t.Fatal("legacy class_policy field unexpectedly accepted")
	}
}

func TestCatalogRejectsMalformedDefinitions(t *testing.T) {
	zeroInterval := uint32(0)
	zeroRange := float32(0)
	negativeRange := float32(-1)
	nanRange := float32(math.NaN())
	infRange := float32(math.Inf(1))
	weaponTypes := []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword}}
	validWeapon := Definition{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}}}
	for name, def := range map[string]CatalogDefinition{
		"duplicate_item":           {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{validWeapon, validWeapon}},
		"duplicate_weapon_type":    {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword}, {WeaponType: WeaponTypeOneHandSword}}, Items: []Definition{validWeapon}},
		"zero_type_interval":       {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackIntervalMS: &zeroInterval}}, Items: []Definition{validWeapon}},
		"zero_type_range":          {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackRange: &zeroRange}}, Items: []Definition{validWeapon}},
		"negative_type_range":      {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackRange: &negativeRange}}, Items: []Definition{validWeapon}},
		"nan_type_range":           {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackRange: &nanRange}}, Items: []Definition{validWeapon}},
		"infinite_type_range":      {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackRange: &infRange}}, Items: []Definition{validWeapon}},
		"invalid_damage_attribute": {Revision: "x", WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword, BasicAttackDamageAttribute: characterstats.Intelligence}}, Items: []Definition{validWeapon}},
		"unknown_weapon_type":      {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: &Weapon{WeaponType: WeaponType("unknown"), SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}}}}},
		"weapon_in_offhand":        {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotOffHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: validWeapon.Weapon}}},
		"invalid_range":            {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{3, 2}, LargeDamage: DamageRange{1, 2}}}}},
		"empty_material":           {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Weapon: validWeapon.Weapon}}},
		"zero_weight":              {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Material: "iron", Weapon: validWeapon.Weapon}}},
		"missing_tier":             {Revision: "x", WeaponTypes: weaponTypes, Items: []Definition{{ItemArchetypeID: "item_test", Kind: KindWeapon, Slot: SlotMainHand, Weight: 1, Material: "iron", Weapon: validWeapon.Weapon}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(def); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
