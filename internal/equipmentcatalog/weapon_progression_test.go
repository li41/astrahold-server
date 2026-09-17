package equipmentcatalog

import "testing"

type weaponTierExpectation struct {
	weaponType WeaponType
	weight uint32
	midMaterial MaterialID
	highMaterial MaterialID
	accuracy int32
	midSmall, midLarge DamageRange
	highSmall, highLarge DamageRange
	midExtra, highExtra uint32
}

func TestDefaultCatalogLocksThreeTierWeaponProgression(t *testing.T) {
	catalog, err := Default()
	if err != nil { t.Fatal(err) }
	if got := catalog.Revision(); got != "weapon-progression-v1" { t.Fatalf("revision=%q", got) }
	if got := len(catalog.byItem); got != 62 { t.Fatalf("catalog item count=%d want=62", got) }

	cases := []weaponTierExpectation{
		{WeaponTypeOneHandSword, 7, MaterialSteel, MaterialStarsteel, 0, DamageRange{9,12}, DamageRange{9,11}, DamageRange{12,15}, DamageRange{11,14}, 0, 0},
		{WeaponTypeDagger, 3, MaterialSilver, MaterialStarsteel, 2, DamageRange{6,8}, DamageRange{5,7}, DamageRange{8,10}, DamageRange{7,9}, 0, 0},
		{WeaponTypeOneHandAxe, 10, MaterialSteel, MaterialStarsteel, -1, DamageRange{7,10}, DamageRange{12,16}, DamageRange{9,13}, DamageRange{15,21}, 0, 0},
		{WeaponTypeOneHandSpear, 8, MaterialSteel, MaterialStarsteel, 1, DamageRange{10,12}, DamageRange{13,15}, DamageRange{12,16}, DamageRange{16,20}, 0, 0},
		{WeaponTypeWarhammer, 10, MaterialSteel, MaterialStarsteel, -1, DamageRange{11,14}, DamageRange{11,14}, DamageRange{14,18}, DamageRange{14,18}, 0, 0},
		{WeaponTypeMorningStar, 10, MaterialSteel, MaterialStarsteel, 0, DamageRange{11,14}, DamageRange{11,14}, DamageRange{14,18}, DamageRange{14,18}, 0, 0},
		{WeaponTypeMace, 9, MaterialSteel, MaterialStarsteel, 1, DamageRange{9,11}, DamageRange{9,11}, DamageRange{11,14}, DamageRange{11,14}, 2, 3},
		{WeaponTypeTwoHandSword, 12, MaterialSteel, MaterialStarsteel, 0, DamageRange{16,22}, DamageRange{15,21}, DamageRange{20,29}, DamageRange{19,27}, 0, 0},
		{WeaponTypeTwoHandAxe, 14, MaterialSteel, MaterialStarsteel, -2, DamageRange{17,25}, DamageRange{22,28}, DamageRange{22,32}, DamageRange{29,36}, 0, 0},
		{WeaponTypeTwoHandSpear, 11, MaterialSteel, MaterialStarsteel, 0, DamageRange{15,21}, DamageRange{20,24}, DamageRange{20,27}, DamageRange{25,32}, 0, 0},
		{WeaponTypeKnuckles, 4, MaterialSteel, MaterialStarsteel, 2, DamageRange{6,9}, DamageRange{6,8}, DamageRange{7,12}, DamageRange{7,10}, 0, 0},
		{WeaponTypeClaw, 5, MaterialSteel, MaterialStarsteel, 1, DamageRange{7,11}, DamageRange{7,10}, DamageRange{9,14}, DamageRange{9,13}, 0, 0},
		{WeaponTypeDualBlades, 8, MaterialSteel, MaterialStarsteel, 0, DamageRange{8,12}, DamageRange{7,10}, DamageRange{11,15}, DamageRange{9,13}, 0, 0},
		{WeaponTypeBow, 4, MaterialReinforcedWood, MaterialStarwood, 1, DamageRange{5,7}, DamageRange{5,7}, DamageRange{8,12}, DamageRange{8,12}, 0, 0},
		{WeaponTypeCrossbow, 8, MaterialSteel, MaterialStarsteel, 2, DamageRange{17,21}, DamageRange{17,21}, DamageRange{22,27}, DamageRange{22,27}, 0, 0},
		{WeaponTypeSling, 2, MaterialReinforcedLeather, MaterialStarhide, 2, DamageRange{10,13}, DamageRange{10,13}, DamageRange{13,17}, DamageRange{13,17}, 0, 0},
		{WeaponTypeStaff, 5, MaterialRunewood, MaterialStarwood, 0, DamageRange{7,10}, DamageRange{7,10}, DamageRange{9,13}, DamageRange{9,13}, 0, 0},
	}
	if len(cases) != 17 { t.Fatalf("weapon lines=%d", len(cases)) }
	for _, tc := range cases {
		for _, tierCase := range []struct{
			tier Tier
			prefix string
			material MaterialID
			small, large DamageRange
			extra uint32
		}{
			{TierMid, "item_mid_", tc.midMaterial, tc.midSmall, tc.midLarge, tc.midExtra},
			{TierHigh, "item_high_", tc.highMaterial, tc.highSmall, tc.highLarge, tc.highExtra},
		} {
			id := tierCase.prefix + string(tc.weaponType)
			item, ok := catalog.Resolve(id)
			if !ok { t.Fatalf("missing %s", id) }
			if item.Kind != KindWeapon || item.Slot != SlotMainHand || item.Tier != tierCase.tier || item.Weapon == nil { t.Fatalf("invalid %s: %#v", id, item) }
			if item.Weight != tc.weight || item.Material != tierCase.material || item.Weapon.WeaponType != tc.weaponType || item.Weapon.SmallDamage != tierCase.small || item.Weapon.LargeDamage != tierCase.large || item.Weapon.ExtraDamage != tierCase.extra || item.Weapon.AccuracyModifier != tc.accuracy {
				t.Fatalf("unexpected %s: %#v", id, item)
			}
			hands, ok := catalog.HandRequirementForItem(id)
			if !ok { t.Fatalf("missing hand requirement for %s", id) }
			wantHands, _ := HandRequirementForWeaponType(tc.weaponType)
			if hands != wantHands { t.Fatalf("%s hands=%q want=%q", id, hands, wantHands) }
		}
	}
}

func TestDefaultStaffStaticMagicPowerProgression(t *testing.T) {
	catalog, err := Default()
	if err != nil { t.Fatal(err) }
	cases := map[string]uint32{
		"item_apprentice_wood_staff": 1,
		"item_mid_staff": 2,
		"item_high_staff": 4,
	}
	for id, want := range cases {
		modifiers, ok := catalog.StaticModifiersForItem(id)
		if !ok || len(modifiers) != 1 || modifiers[0] != (StaticModifier{ID: StaticMagicPower, Value: want}) {
			t.Fatalf("%s static modifiers=%#v ok=%v", id, modifiers, ok)
		}
	}
	for _, id := range []string{"item_mid_one_hand_sword", "item_high_bow", "item_high_mace"} {
		modifiers, ok := catalog.StaticModifiersForItem(id)
		if !ok || len(modifiers) != 0 { t.Fatalf("%s unexpected static modifiers=%#v", id, modifiers) }
	}
}
