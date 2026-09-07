package weaponeffect

import "testing"

func TestDefaultCatalogResolvesManaSiphonStaff(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := catalog.Resolve("item_mana_siphon_staff")
	if !ok {
		t.Fatal("mana siphon staff not found")
	}
	if definition.ManaRestoreOnDamageHit != 5 {
		t.Fatalf("mana restore = %d, want 5", definition.ManaRestoreOnDamageHit)
	}
}

func TestCatalogRejectsInvalidWeaponEffects(t *testing.T) {
	tests := []CatalogDefinition{
		{Revision: "", Weapons: []Definition{{ItemArchetypeID: "item_staff", ManaRestoreOnDamageHit: 5}}},
		{Revision: "r1", Weapons: nil},
		{Revision: "r1", Weapons: []Definition{{ItemArchetypeID: "", ManaRestoreOnDamageHit: 5}}},
		{Revision: "r1", Weapons: []Definition{{ItemArchetypeID: "item_staff", ManaRestoreOnDamageHit: 0}}},
		{Revision: "r1", Weapons: []Definition{
			{ItemArchetypeID: "item_staff", ManaRestoreOnDamageHit: 5},
			{ItemArchetypeID: "item_staff", ManaRestoreOnDamageHit: 6},
		}},
	}
	for index, definition := range tests {
		if _, err := New(definition); err == nil {
			t.Fatalf("case %d: expected invalid catalog", index)
		}
	}
}
