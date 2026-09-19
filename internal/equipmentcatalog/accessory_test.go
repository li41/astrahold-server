package equipmentcatalog

import "testing"

func TestCatalogAcceptsAccessorySlots(t *testing.T) {
	for _, tc := range []struct {
		name string
		slot Slot
	}{
		{"necklace", SlotNecklace},
		{"ring", SlotRing},
		{"belt", SlotBelt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := New(CatalogDefinition{
				Revision: "accessory-test",
				Items: []Definition{{
					ItemArchetypeID: "item_test_accessory",
					Kind:            KindAccessory,
					Slot:            tc.slot,
					Tier:            TierLow,
					Weight:          1,
					Material:        "iron",
				}},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			item, ok := catalog.Resolve("item_test_accessory")
			if !ok || item.Kind != KindAccessory || item.Slot != tc.slot {
				t.Fatalf("Resolve=%+v ok=%v", item, ok)
			}
		})
	}
}

func TestCatalogRejectsAccessoryInNonAccessorySlot(t *testing.T) {
	for _, slot := range []Slot{SlotMainHand, SlotOffHand, SlotHelmet, SlotChest, SlotGloves, SlotLegs, SlotBoots} {
		_, err := New(CatalogDefinition{
			Revision: "accessory-invalid-slot-test",
			Items: []Definition{{
				ItemArchetypeID: "item_test_accessory",
				Kind:            KindAccessory,
				Slot:            slot,
				Tier:            TierLow,
				Weight:          1,
				Material:        "iron",
			}},
		})
		if err == nil {
			t.Fatalf("accessory slot %q unexpectedly accepted", slot)
		}
	}
}

func TestCatalogRejectsAccessoryCombatPayload(t *testing.T) {
	weapon := &Weapon{WeaponType: WeaponTypeOneHandSword, SmallDamage: DamageRange{1, 1}, LargeDamage: DamageRange{1, 1}}
	shield := &Shield{PhysicalDefense: 1}
	for name, item := range map[string]Definition{
		"weapon payload": {ItemArchetypeID: "item_test_accessory", Kind: KindAccessory, Slot: SlotNecklace, Tier: TierLow, Weight: 1, Material: "iron", Weapon: weapon},
		"shield payload": {ItemArchetypeID: "item_test_accessory", Kind: KindAccessory, Slot: SlotBelt, Tier: TierLow, Weight: 1, Material: "iron", Shield: shield},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(CatalogDefinition{Revision: "accessory-payload-test", Items: []Definition{item}}); err == nil {
				t.Fatal("accessory combat payload unexpectedly accepted")
			}
		})
	}
}
