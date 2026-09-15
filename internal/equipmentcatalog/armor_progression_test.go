package equipmentcatalog

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
)

func TestDefaultArmorProgressionHasFortyFivePieces(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(catalog.armorByItem); got != 45 {
		t.Fatalf("armor count=%d want=45", got)
	}
	if got := len(catalog.UnitWeights()); got != 107 {
		t.Fatalf("equipment weight entries=%d want=107", got)
	}

	type expected struct {
		id       string
		slot     Slot
		tier     Tier
		class    ArmorClass
		setID    SetID
		weight   uint32
		physical uint32
		magic    uint32
		reqStat  characterstats.ID
		reqMin   uint32
	}
	cases := []expected{
		{"item_garrison_steel_helm", SlotHelmet, TierMid, ArmorClassHeavy, "set_garrison_steel", 5, 2, 1, "", 0},
		{"item_garrison_steel_cuirass", SlotChest, TierMid, ArmorClassHeavy, "set_garrison_steel", 10, 4, 1, "", 0},
		{"item_garrison_steel_gauntlets", SlotGloves, TierMid, ArmorClassHeavy, "set_garrison_steel", 4, 1, 0, "", 0},
		{"item_garrison_steel_greaves", SlotLegs, TierMid, ArmorClassHeavy, "set_garrison_steel", 8, 3, 1, "", 0},
		{"item_garrison_steel_boots", SlotBoots, TierMid, ArmorClassHeavy, "set_garrison_steel", 4, 2, 1, "", 0},
		{"item_windchaser_cap", SlotHelmet, TierMid, ArmorClassLeather, "set_windchaser_huntgear", 2, 1, 1, "", 0},
		{"item_windchaser_armor", SlotChest, TierMid, ArmorClassLeather, "set_windchaser_huntgear", 5, 3, 2, "", 0},
		{"item_windchaser_gloves", SlotGloves, TierMid, ArmorClassLeather, "set_windchaser_huntgear", 2, 1, 1, "", 0},
		{"item_windchaser_leggings", SlotLegs, TierMid, ArmorClassLeather, "set_windchaser_huntgear", 4, 2, 1, "", 0},
		{"item_windchaser_boots", SlotBoots, TierMid, ArmorClassLeather, "set_windchaser_huntgear", 2, 1, 1, "", 0},
		{"item_arcane_rune_hood", SlotHelmet, TierMid, ArmorClassCloth, "set_arcane_rune_robes", 1, 1, 2, "", 0},
		{"item_arcane_rune_robe", SlotChest, TierMid, ArmorClassCloth, "set_arcane_rune_robes", 3, 1, 4, "", 0},
		{"item_arcane_rune_gloves", SlotGloves, TierMid, ArmorClassCloth, "set_arcane_rune_robes", 1, 0, 1, "", 0},
		{"item_arcane_rune_trousers", SlotLegs, TierMid, ArmorClassCloth, "set_arcane_rune_robes", 2, 1, 3, "", 0},
		{"item_arcane_rune_boots", SlotBoots, TierMid, ArmorClassCloth, "set_arcane_rune_robes", 1, 1, 2, "", 0},
		{"item_starforged_bastion_helm", SlotHelmet, TierHigh, ArmorClassHeavy, "set_starforged_bastion", 6, 3, 1, characterstats.Constitution, 18},
		{"item_starforged_bastion_cuirass", SlotChest, TierHigh, ArmorClassHeavy, "set_starforged_bastion", 12, 5, 2, characterstats.Constitution, 18},
		{"item_starforged_bastion_gauntlets", SlotGloves, TierHigh, ArmorClassHeavy, "set_starforged_bastion", 5, 2, 1, characterstats.Constitution, 18},
		{"item_starforged_bastion_greaves", SlotLegs, TierHigh, ArmorClassHeavy, "set_starforged_bastion", 10, 4, 1, characterstats.Constitution, 18},
		{"item_starforged_bastion_boots", SlotBoots, TierHigh, ArmorClassHeavy, "set_starforged_bastion", 5, 2, 1, characterstats.Constitution, 18},
		{"item_starshadow_hunter_cap", SlotHelmet, TierHigh, ArmorClassLeather, "set_starshadow_huntgear", 2, 2, 1, characterstats.Agility, 18},
		{"item_starshadow_hunter_armor", SlotChest, TierHigh, ArmorClassLeather, "set_starshadow_huntgear", 6, 3, 3, characterstats.Agility, 18},
		{"item_starshadow_hunter_gloves", SlotGloves, TierHigh, ArmorClassLeather, "set_starshadow_huntgear", 2, 1, 1, characterstats.Agility, 18},
		{"item_starshadow_hunter_leggings", SlotLegs, TierHigh, ArmorClassLeather, "set_starshadow_huntgear", 5, 2, 2, characterstats.Agility, 18},
		{"item_starshadow_hunter_boots", SlotBoots, TierHigh, ArmorClassLeather, "set_starshadow_huntgear", 2, 2, 1, characterstats.Agility, 18},
		{"item_starlight_hood", SlotHelmet, TierHigh, ArmorClassCloth, "set_starlight_robes", 1, 1, 3, characterstats.Intelligence, 18},
		{"item_starlight_robe", SlotChest, TierHigh, ArmorClassCloth, "set_starlight_robes", 4, 2, 5, characterstats.Intelligence, 18},
		{"item_starlight_gloves", SlotGloves, TierHigh, ArmorClassCloth, "set_starlight_robes", 1, 1, 2, characterstats.Intelligence, 18},
		{"item_starlight_trousers", SlotLegs, TierHigh, ArmorClassCloth, "set_starlight_robes", 2, 1, 4, characterstats.Intelligence, 18},
		{"item_starlight_boots", SlotBoots, TierHigh, ArmorClassCloth, "set_starlight_robes", 1, 1, 2, characterstats.Intelligence, 18},
	}
	for _, want := range cases {
		got, ok := catalog.Resolve(want.id)
		if !ok {
			t.Fatalf("missing %s", want.id)
		}
		if got.Kind != KindArmor || got.Slot != want.slot || got.Tier != want.tier || got.ArmorClass != want.class || got.SetID != want.setID || got.Weight != want.weight {
			t.Fatalf("%s metadata=%#v", want.id, got)
		}
		if modifierValue(got.StaticModifiers, StaticPhysicalDefense) != want.physical || modifierValue(got.StaticModifiers, StaticMagicDefense) != want.magic {
			t.Fatalf("%s modifiers=%#v", want.id, got.StaticModifiers)
		}
		if want.reqStat == "" {
			if len(got.BaseRequirements) != 0 {
				t.Fatalf("%s requirements=%#v want none", want.id, got.BaseRequirements)
			}
		} else if len(got.BaseRequirements) != 1 || got.BaseRequirements[0].Stat != want.reqStat || got.BaseRequirements[0].Minimum != want.reqMin {
			t.Fatalf("%s requirements=%#v", want.id, got.BaseRequirements)
		}
	}
}

func TestArmorSetBonusesAreDerivedCumulatively(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{
		"item_garrison_steel_helm",
		"item_garrison_steel_cuirass",
		"item_garrison_steel_gauntlets",
		"item_garrison_steel_greaves",
		"item_garrison_steel_boots",
	}
	two, err := catalog.SetStaticModifiersForEquipped(ids[:2])
	if err != nil {
		t.Fatal(err)
	}
	if modifierValue(two, StaticMaxHP) != 40 || modifierValue(two, StaticStrength) != 0 || modifierValue(two, StaticPhysicalDefense) != 0 {
		t.Fatalf("2-piece bonuses=%#v", two)
	}
	five, err := catalog.SetStaticModifiersForEquipped(ids)
	if err != nil {
		t.Fatal(err)
	}
	if modifierValue(five, StaticMaxHP) != 40 || modifierValue(five, StaticStrength) != 2 || modifierValue(five, StaticPhysicalDefense) != 2 {
		t.Fatalf("5-piece cumulative bonuses=%#v", five)
	}
}

func TestHighArmorRequirementsUseDurableBaseStats(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := catalog.Resolve("item_starshadow_hunter_armor")
	if !ok {
		t.Fatal("starshadow armor missing")
	}
	base := characterstats.DefaultPrimary()
	if definition.BaseRequirementsMet(base) {
		t.Fatal("base agility 10 satisfied agility 18 requirement")
	}
	base.Agility = 18
	if !definition.BaseRequirementsMet(base) {
		t.Fatal("base agility 18 did not satisfy requirement")
	}
}

func modifierValue(modifiers []StaticModifier, id StaticModifierID) uint32 {
	var total uint32
	for _, modifier := range modifiers {
		if modifier.ID == id {
			total += modifier.Value
		}
	}
	return total
}
