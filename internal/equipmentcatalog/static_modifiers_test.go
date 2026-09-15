package equipmentcatalog

import (
	"errors"
	"testing"
)

func TestCatalogCanonicalizesAndDefensivelyCopiesStaticModifiers(t *testing.T) {
	catalog, err := New(CatalogDefinition{
		Revision: "static-modifier-test",
		WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword}},
		Items: []Definition{{
			ItemArchetypeID: "item_static_test",
			Kind: KindWeapon,
			Slot: SlotMainHand,
			Tier: TierLow,
			Weight: 1,
			Material: "iron",
			StaticModifiers: []StaticModifier{
				{ID: StaticMagicPower, Value: 3},
				{ID: StaticStrength, Value: 2},
			},
			Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := catalog.Resolve("item_static_test")
	if !ok || len(item.StaticModifiers) != 2 {
		t.Fatalf("static modifiers missing: %#v", item)
	}
	if item.StaticModifiers[0] != (StaticModifier{ID: StaticMagicPower, Value: 3}) || item.StaticModifiers[1] != (StaticModifier{ID: StaticStrength, Value: 2}) {
		t.Fatalf("canonical static modifiers=%#v", item.StaticModifiers)
	}
	item.StaticModifiers[0].Value = 99
	again, ok := catalog.StaticModifiersForItem("item_static_test")
	if !ok || again[0].Value != 3 {
		t.Fatalf("catalog static modifiers mutated through caller: %#v", again)
	}
}

func TestCatalogRejectsInvalidStaticModifiers(t *testing.T) {
	base := Definition{
		ItemArchetypeID: "item_static_test",
		Kind: KindWeapon,
		Slot: SlotMainHand,
		Tier: TierLow,
		Weight: 1,
		Material: "iron",
		Weapon: &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{1, 2}, LargeDamage: DamageRange{1, 2}},
	}
	cases := map[string][]StaticModifier{
		"unknown": {{ID: "unknown", Value: 1}},
		"zero": {{ID: StaticMagicPower, Value: 0}},
		"duplicate": {{ID: StaticMagicPower, Value: 1}, {ID: StaticMagicPower, Value: 2}},
	}
	for name, modifiers := range cases {
		t.Run(name, func(t *testing.T) {
			item := base
			item.StaticModifiers = modifiers
			_, err := New(CatalogDefinition{
				Revision: "static-modifier-invalid",
				WeaponTypes: []WeaponTypeDefinition{{WeaponType: WeaponTypeOneHandSword}},
				Items: []Definition{item},
			})
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
