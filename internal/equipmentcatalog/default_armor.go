package equipmentcatalog

// Low-tier armor is deliberately archetype-only. It introduces no drop/shop/economy source; it
// only establishes the formal five armor slots and three baseline armor profiles.
func defaultLowTierArmor() []Definition {
	return []Definition{
		armor("item_low_cloth_helmet", SlotHelmet, ArmorClassCloth, 1, MaterialCloth, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_cloth_chest", SlotChest, ArmorClassCloth, 2, MaterialCloth, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_cloth_gloves", SlotGloves, ArmorClassCloth, 1, MaterialCloth, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_cloth_legs", SlotLegs, ArmorClassCloth, 2, MaterialCloth, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_cloth_boots", SlotBoots, ArmorClassCloth, 1, MaterialCloth, StaticModifier{ID: StaticMagicDefense, Value: 1}),

		armor("item_low_leather_helmet", SlotHelmet, ArmorClassLeather, 2, MaterialLeather, StaticModifier{ID: StaticPhysicalDefense, Value: 1}, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_leather_chest", SlotChest, ArmorClassLeather, 4, MaterialLeather, StaticModifier{ID: StaticPhysicalDefense, Value: 1}, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_leather_gloves", SlotGloves, ArmorClassLeather, 2, MaterialLeather, StaticModifier{ID: StaticPhysicalDefense, Value: 1}, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_leather_legs", SlotLegs, ArmorClassLeather, 3, MaterialLeather, StaticModifier{ID: StaticPhysicalDefense, Value: 1}, StaticModifier{ID: StaticMagicDefense, Value: 1}),
		armor("item_low_leather_boots", SlotBoots, ArmorClassLeather, 2, MaterialLeather, StaticModifier{ID: StaticPhysicalDefense, Value: 1}, StaticModifier{ID: StaticMagicDefense, Value: 1}),

		armor("item_low_heavy_helmet", SlotHelmet, ArmorClassHeavy, 4, MaterialIron, StaticModifier{ID: StaticPhysicalDefense, Value: 2}),
		armor("item_low_heavy_chest", SlotChest, ArmorClassHeavy, 8, MaterialIron, StaticModifier{ID: StaticPhysicalDefense, Value: 2}),
		armor("item_low_heavy_gloves", SlotGloves, ArmorClassHeavy, 3, MaterialIron, StaticModifier{ID: StaticPhysicalDefense, Value: 2}),
		armor("item_low_heavy_legs", SlotLegs, ArmorClassHeavy, 6, MaterialIron, StaticModifier{ID: StaticPhysicalDefense, Value: 2}),
		armor("item_low_heavy_boots", SlotBoots, ArmorClassHeavy, 4, MaterialIron, StaticModifier{ID: StaticPhysicalDefense, Value: 2}),
	}
}

func armor(id string, slot Slot, class ArmorClass, weight uint32, material MaterialID, modifiers ...StaticModifier) Definition {
	return Definition{
		ItemArchetypeID: id,
		Kind:            KindArmor,
		Slot:            slot,
		Tier:            TierLow,
		Weight:          weight,
		Material:        material,
		ArmorClass:      class,
		StaticModifiers: modifiers,
	}
}
