package equipmentcatalog

import "testing"

func TestFormalMaterialIDsAreStableAndExhaustive(t *testing.T) {
	ids := []MaterialID{
		MaterialIron,
		MaterialSteel,
		MaterialSilver,
		MaterialStarsteel,
		MaterialWood,
		MaterialReinforcedWood,
		MaterialRunewood,
		MaterialStarwood,
		MaterialLeather,
		MaterialReinforcedLeather,
		MaterialStarhide,
		MaterialCloth,
		MaterialRunedCloth,
		MaterialStarweave,
	}
	if len(ids) != 14 {
		t.Fatalf("material id count=%d want=14", len(ids))
	}
	seen := make(map[MaterialID]struct{}, len(ids))
	for _, id := range ids {
		if !id.Valid() {
			t.Fatalf("formal material %q is not valid", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate material id %q", id)
		}
		seen[id] = struct{}{}
	}
	for _, invalid := range []MaterialID{"", "iron_wood", "wood_iron", "silvered", "stone"} {
		if invalid.Valid() {
			t.Fatalf("legacy/unknown material %q unexpectedly valid", invalid)
		}
	}
}

func TestDefaultCatalogLocksAll107PrimaryMaterials(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(defaultProductionMaterialByItem); got != 107 {
		t.Fatalf("formal material map entries=%d want=107", got)
	}
	if got := len(catalog.UnitWeights()); got != 107 {
		t.Fatalf("production equipment entries=%d want=107", got)
	}
	for id, want := range defaultProductionMaterialByItem {
		got, ok := catalog.Resolve(id)
		if !ok {
			t.Fatalf("formal material item missing: %s", id)
		}
		if got.Material != want {
			t.Fatalf("%s material=%q want=%q", id, got.Material, want)
		}
		if !got.Material.Valid() {
			t.Fatalf("%s has invalid material %q", id, got.Material)
		}
	}
}

func TestCatalogRejectsLegacyCompoundMaterialID(t *testing.T) {
	catalog := CatalogDefinition{
		Revision: "material-validation-test",
		WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword}},
		Items: []Definition{{
			ItemArchetypeID: "item_test",
			Kind: KindWeapon,
			Slot: SlotMainHand,
			Tier: TierLow,
			Weight: 1,
			Material: MaterialID("iron_wood"),
			Weapon: &Weapon{
				WeaponType: WeaponTypeOneHandSword,
				SmallDamage: DamageRange{Min: 1, Max: 2},
				LargeDamage: DamageRange{Min: 1, Max: 2},
			},
		}},
	}
	if _, err := New(catalog); err == nil {
		t.Fatal("legacy compound material unexpectedly accepted")
	}
}
