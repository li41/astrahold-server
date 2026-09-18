package equipmentcatalog

import "testing"

func TestDefaultRangedWeaponsSplitBaseDamageForTwoArrowSystem(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		weaponType WeaponType
		damage     DamageRange
	}{
		"item_hunter_shortbow":       {weaponType: WeaponTypeBow, damage: DamageRange{Min: 2, Max: 3}},
		"item_mid_bow":               {weaponType: WeaponTypeBow, damage: DamageRange{Min: 5, Max: 7}},
		"item_high_bow":              {weaponType: WeaponTypeBow, damage: DamageRange{Min: 8, Max: 12}},
		"item_hunter_light_crossbow": {weaponType: WeaponTypeCrossbow, damage: DamageRange{Min: 6, Max: 7}},
		"item_mid_crossbow":          {weaponType: WeaponTypeCrossbow, damage: DamageRange{Min: 11, Max: 13}},
		"item_high_crossbow":         {weaponType: WeaponTypeCrossbow, damage: DamageRange{Min: 16, Max: 19}},
	}
	for itemID, expected := range want {
		definition, ok := catalog.Resolve(itemID)
		if !ok || definition.Weapon == nil {
			t.Fatalf("ranged weapon %q missing", itemID)
		}
		if definition.Weapon.WeaponType != expected.weaponType {
			t.Fatalf("%q weapon type=%q want=%q", itemID, definition.Weapon.WeaponType, expected.weaponType)
		}
		if definition.Weapon.SmallDamage != expected.damage || definition.Weapon.LargeDamage != expected.damage {
			t.Fatalf("%q small=%+v large=%+v want %+v", itemID, definition.Weapon.SmallDamage, definition.Weapon.LargeDamage, expected.damage)
		}
	}
}
