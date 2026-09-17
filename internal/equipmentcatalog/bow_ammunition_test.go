package equipmentcatalog

import "testing"

func TestDefaultBowsSplitBaseDamageForTwoArrowSystem(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]DamageRange{
		"item_hunter_shortbow": {Min: 2, Max: 3},
		"item_mid_bow":         {Min: 5, Max: 7},
		"item_high_bow":        {Min: 8, Max: 12},
	}
	for itemID, expected := range want {
		definition, ok := catalog.Resolve(itemID)
		if !ok || definition.Weapon == nil {
			t.Fatalf("bow %q missing", itemID)
		}
		if definition.Weapon.WeaponType != WeaponTypeBow {
			t.Fatalf("bow %q weapon type=%q", itemID, definition.Weapon.WeaponType)
		}
		if definition.Weapon.SmallDamage != expected || definition.Weapon.LargeDamage != expected {
			t.Fatalf("bow %q small=%+v large=%+v want %+v", itemID, definition.Weapon.SmallDamage, definition.Weapon.LargeDamage, expected)
		}
	}
}
