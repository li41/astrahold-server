package equipmentcatalog

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
)

func TestDefaultCatalogHasLowTierRepresentativeForEveryFormalWeaponType(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id              string
		weaponType      WeaponType
		material        string
		small           DamageRange
		large           DamageRange
		extra           uint32
		accuracy        int32
		weight          uint32
		intervalMS      uint32
		damageAttribute characterstats.ID
		attackRange     float32
		hands           HandRequirement
	}{
		{"item_militia_iron_sword", WeaponTypeOneHandSword, "iron", DamageRange{6, 9}, DamageRange{6, 8}, 0, 0, 7, 900, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_iron_dagger", WeaponTypeDagger, "iron", DamageRange{4, 6}, DamageRange{3, 5}, 0, 2, 3, 650, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_militia_battle_axe", WeaponTypeOneHandAxe, "iron_wood", DamageRange{5, 8}, DamageRange{8, 12}, 0, -1, 10, 1050, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_militia_iron_spear", WeaponTypeOneHandSpear, "iron_wood", DamageRange{7, 9}, DamageRange{9, 11}, 0, 1, 8, 1000, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_iron_warhammer", WeaponTypeWarhammer, "iron_wood", DamageRange{8, 11}, DamageRange{8, 11}, 0, -1, 10, 1150, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_iron_morning_star", WeaponTypeMorningStar, "iron_chain", DamageRange{8, 11}, DamageRange{8, 11}, 0, 0, 10, 1200, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_iron_war_mace", WeaponTypeMace, "iron_wood", DamageRange{6, 9}, DamageRange{6, 9}, 1, 1, 9, 1050, characterstats.Strength, 0, HandRequirementOneHand},
		{"item_two_hand_iron_sword", WeaponTypeTwoHandSword, "iron", DamageRange{11, 16}, DamageRange{10, 15}, 0, 0, 12, 1350, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_two_hand_battle_axe", WeaponTypeTwoHandAxe, "iron_wood", DamageRange{12, 18}, DamageRange{16, 20}, 0, -2, 14, 1500, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_long_iron_spear", WeaponTypeTwoHandSpear, "iron_wood", DamageRange{11, 15}, DamageRange{14, 18}, 0, 0, 11, 1300, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_iron_knuckles", WeaponTypeKnuckles, "iron_leather", DamageRange{4, 7}, DamageRange{4, 6}, 0, 2, 4, 600, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_iron_claw", WeaponTypeClaw, "iron_leather", DamageRange{5, 8}, DamageRange{5, 7}, 0, 1, 5, 700, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_militia_dual_blades", WeaponTypeDualBlades, "iron", DamageRange{6, 8}, DamageRange{5, 7}, 0, 0, 8, 700, characterstats.Strength, 0, HandRequirementTwoHand},
		{"item_hunter_shortbow", WeaponTypeBow, "wood_leather", DamageRange{8, 11}, DamageRange{8, 11}, 0, 1, 4, 1200, characterstats.Agility, 18, HandRequirementTwoHand},
		{"item_hunter_light_crossbow", WeaponTypeCrossbow, "wood_iron", DamageRange{12, 15}, DamageRange{12, 15}, 0, 2, 8, 1550, characterstats.Agility, 22, HandRequirementTwoHand},
		{"item_leather_sling", WeaponTypeSling, "leather", DamageRange{7, 10}, DamageRange{7, 10}, 0, 2, 2, 1100, characterstats.Agility, 14, HandRequirementOneHand},
		{"item_apprentice_wood_staff", WeaponTypeStaff, "wood", DamageRange{5, 8}, DamageRange{5, 8}, 0, 0, 5, 1250, "", 0, HandRequirementTwoHand},
	}
	if len(cases) != 17 {
		t.Fatalf("representative weapon count = %d, want 17", len(cases))
	}

	seenTypes := make(map[WeaponType]struct{}, len(cases))
	for _, tc := range cases {
		item, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing low-tier representative %s", tc.id)
		}
		if item.Kind != KindWeapon || item.Slot != SlotMainHand || item.Tier != TierLow || item.Weapon == nil {
			t.Fatalf("%s not low-tier main-hand weapon: %#v", tc.id, item)
		}
		if item.Material != tc.material || item.Weight != tc.weight || item.Weapon.WeaponType != tc.weaponType || item.Weapon.SmallDamage != tc.small || item.Weapon.LargeDamage != tc.large || item.Weapon.ExtraDamage != tc.extra || item.Weapon.AccuracyModifier != tc.accuracy {
			t.Fatalf("%s unexpected values: %#v", tc.id, item)
		}
		if _, duplicate := seenTypes[tc.weaponType]; duplicate {
			t.Fatalf("duplicate representative for weapon type %s", tc.weaponType)
		}
		seenTypes[tc.weaponType] = struct{}{}

		interval, authored := catalog.BasicAttackIntervalMSForItem(tc.id)
		if !authored || interval != tc.intervalMS {
			t.Fatalf("%s cadence = %d authored=%v, want %d", tc.id, interval, authored, tc.intervalMS)
		}

		attribute, authored := catalog.BasicAttackDamageAttributeForItem(tc.id)
		if tc.damageAttribute == "" {
			if authored || attribute != "" {
				t.Fatalf("%s damage attribute = %q authored=%v, want none", tc.id, attribute, authored)
			}
		} else if !authored || attribute != tc.damageAttribute {
			t.Fatalf("%s damage attribute = %q authored=%v, want %q", tc.id, attribute, authored, tc.damageAttribute)
		}

		attackRange, authored := catalog.BasicAttackRangeForItem(tc.id)
		if tc.attackRange == 0 {
			if authored || attackRange != 0 {
				t.Fatalf("%s attack range = %v authored=%v, want action fallback", tc.id, attackRange, authored)
			}
		} else if !authored || attackRange != tc.attackRange {
			t.Fatalf("%s attack range = %v authored=%v, want %v", tc.id, attackRange, authored, tc.attackRange)
		}

		hands, authored := catalog.HandRequirementForItem(tc.id)
		if !authored || hands != tc.hands {
			t.Fatalf("%s hand requirement = %q authored=%v, want %q", tc.id, hands, authored, tc.hands)
		}
	}
}

func TestDefaultCatalogKeepsLowTierSwordAlternatives(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"item_light_guard_sword", "item_gladiator_iron_sword"} {
		item, ok := catalog.Resolve(id)
		if !ok || item.Kind != KindWeapon || item.Tier != TierLow || item.Weapon == nil || item.Weapon.WeaponType != WeaponTypeOneHandSword {
			t.Fatalf("low-tier sword alternative %s missing or invalid: %#v", id, item)
		}
	}
}
