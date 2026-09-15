package equipmentcatalog

import "testing"

func TestFormalWeaponTypeHandRequirements(t *testing.T) {
	cases := map[WeaponType]HandRequirement{
		WeaponTypeOneHandSword: HandRequirementOneHand,
		WeaponTypeDagger:       HandRequirementOneHand,
		WeaponTypeOneHandAxe:   HandRequirementOneHand,
		WeaponTypeOneHandSpear: HandRequirementOneHand,
		WeaponTypeWarhammer:    HandRequirementOneHand,
		WeaponTypeMorningStar:  HandRequirementOneHand,
		WeaponTypeMace:         HandRequirementOneHand,
		WeaponTypeSling:        HandRequirementOneHand,
		WeaponTypeTwoHandSword: HandRequirementTwoHand,
		WeaponTypeTwoHandAxe:   HandRequirementTwoHand,
		WeaponTypeTwoHandSpear: HandRequirementTwoHand,
		WeaponTypeKnuckles:     HandRequirementTwoHand,
		WeaponTypeClaw:         HandRequirementTwoHand,
		WeaponTypeDualBlades:   HandRequirementTwoHand,
		WeaponTypeBow:          HandRequirementTwoHand,
		WeaponTypeCrossbow:     HandRequirementTwoHand,
		WeaponTypeStaff:        HandRequirementTwoHand,
	}
	if len(cases) != 17 {
		t.Fatalf("hand requirement case count = %d, want 17", len(cases))
	}
	for weaponType, want := range cases {
		got, ok := HandRequirementForWeaponType(weaponType)
		if !ok || got != want {
			t.Fatalf("%s hand requirement = %q ok=%v, want %q", weaponType, got, ok, want)
		}
	}
	if got, ok := HandRequirementForWeaponType("future_unknown"); ok || got != "" {
		t.Fatalf("unknown hand requirement = %q ok=%v", got, ok)
	}
}

func TestCatalogResolvesHandRequirementFromWeaponType(t *testing.T) {
	catalog, err := New(CatalogDefinition{
		Revision: "hand-requirement-test",
		WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeTwoHandSword}},
		Items: []Definition{
			{ItemArchetypeID: "item_test_two_hand", Kind: KindWeapon, Slot: SlotMainHand, Tier: TierLow, Weight: 1, Material: "iron", Weapon: &Weapon{WeaponType: WeaponTypeTwoHandSword, SmallDamage: DamageRange{1, 1}, LargeDamage: DamageRange{1, 1}}},
			{ItemArchetypeID: "item_test_shield", Kind: KindShield, Slot: SlotOffHand, Tier: TierLow, Weight: 1, Material: "wood", Shield: &Shield{PhysicalDefense: 1}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := catalog.HandRequirementForItem("item_test_two_hand"); !ok || got != HandRequirementTwoHand {
		t.Fatalf("two-hand item requirement = %q ok=%v", got, ok)
	}
	if got, ok := catalog.HandRequirementForItem("item_test_shield"); ok || got != "" {
		t.Fatalf("shield requirement = %q ok=%v", got, ok)
	}
}
