package equipmentcatalog

import "testing"

func TestDefaultCatalogExistingItemsAreLowTier(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	ids := []string{
		"item_militia_iron_sword",
		"item_light_guard_sword",
		"item_gladiator_iron_sword",
		"item_militia_battle_axe",
		"item_iron_war_mace",
		"item_iron_rim_round_shield",
		"item_guard_shield",
		"item_runed_square_shield",
	}
	for _, id := range ids {
		item, ok := catalog.Resolve(id)
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if item.Tier != TierLow {
			t.Fatalf("%s tier=%q, want %q", id, item.Tier, TierLow)
		}
	}
}

func TestAuthoredCatalogRejectsMissingTier(t *testing.T) {
	data := []byte(`{
		"revision":"test",
		"weapon_types":[{"weapon_type":"one_hand_sword"}],
		"items":[{
			"item_archetype_id":"item_test",
			"kind":"weapon",
			"slot":"main_hand",
			"weight":1,
			"material":"iron",
			"weapon":{"weapon_type":"one_hand_sword","small_damage":{"min":1,"max":1},"large_damage":{"min":1,"max":1},"extra_damage":0,"accuracy_modifier":0}
		}]
	}`)
	if _, err := Load(data); err == nil {
		t.Fatal("authored catalog with missing tier unexpectedly valid")
	}
}

func TestCatalogRejectsUnknownTierAndAcceptsMidTier(t *testing.T) {
	base := CatalogDefinition{
		Revision: "test",
		WeaponTypes: []WeaponTypeDefinition{{
			WeaponType: WeaponTypeOneHandSword,
		}},
		Items: []Definition{{
			ItemArchetypeID: "item_test",
			Kind:            KindWeapon,
			Slot:            SlotMainHand,
			Weight:          1,
			Material:        "iron",
			Weapon: &Weapon{
				WeaponType:  WeaponTypeOneHandSword,
				SmallDamage: DamageRange{Min: 1, Max: 1},
				LargeDamage: DamageRange{Min: 1, Max: 1},
			},
		}},
	}
	base.Items[0].Tier = Tier("mythic")
	if _, err := New(base); err == nil {
		t.Fatal("catalog with unknown tier unexpectedly valid")
	}
	base.Items[0].Tier = TierMid
	if _, err := New(base); err != nil {
		t.Fatalf("catalog with mid tier should be structurally valid: %v", err)
	}
}
