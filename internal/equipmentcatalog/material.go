package equipmentcatalog

import "strings"

// MaterialID is the stable Server gameplay identity for an equipment archetype's primary material.
// It is independent from Client mesh/PBR/material names and never stores an asset path.
type MaterialID string

const (
	MaterialIron              MaterialID = "iron"
	MaterialSteel             MaterialID = "steel"
	MaterialSilver            MaterialID = "silver"
	MaterialStarsteel         MaterialID = "starsteel"
	MaterialWood              MaterialID = "wood"
	MaterialReinforcedWood    MaterialID = "reinforced_wood"
	MaterialRunewood          MaterialID = "runewood"
	MaterialStarwood          MaterialID = "starwood"
	MaterialLeather           MaterialID = "leather"
	MaterialReinforcedLeather MaterialID = "reinforced_leather"
	MaterialStarhide          MaterialID = "starhide"
	MaterialCloth             MaterialID = "cloth"
	MaterialRunedCloth        MaterialID = "runed_cloth"
	MaterialStarweave         MaterialID = "starweave"
)

func (id MaterialID) Valid() bool {
	switch id {
	case MaterialIron,
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
		MaterialStarweave:
		return true
	default:
		return false
	}
}

func canonicalMaterialID(id MaterialID) MaterialID {
	return MaterialID(strings.TrimSpace(string(id)))
}

// defaultProductionMaterialByItem is the exact primary-material contract for the current 107
// production equipment archetypes. Historical compound strings in default.json are deliberately
// ignored here instead of being inferred or parsed; every production ItemArchetypeID is mapped
// explicitly so crafting/presentation structure cannot become gameplay material truth by accident.
var defaultProductionMaterialByItem = map[string]MaterialID{
	"item_militia_iron_sword": MaterialIron,
	"item_light_guard_sword": MaterialIron,
	"item_gladiator_iron_sword": MaterialIron,
	"item_iron_dagger": MaterialIron,
	"item_militia_battle_axe": MaterialIron,
	"item_militia_iron_spear": MaterialIron,
	"item_iron_warhammer": MaterialIron,
	"item_iron_morning_star": MaterialIron,
	"item_iron_war_mace": MaterialIron,
	"item_two_hand_iron_sword": MaterialIron,
	"item_two_hand_battle_axe": MaterialIron,
	"item_long_iron_spear": MaterialIron,
	"item_iron_knuckles": MaterialIron,
	"item_iron_claw": MaterialIron,
	"item_militia_dual_blades": MaterialIron,
	"item_hunter_shortbow": MaterialWood,
	"item_hunter_light_crossbow": MaterialWood,
	"item_leather_sling": MaterialLeather,
	"item_apprentice_wood_staff": MaterialWood,
	"item_mid_one_hand_sword": MaterialSteel,
	"item_mid_dagger": MaterialSilver,
	"item_mid_one_hand_axe": MaterialSteel,
	"item_mid_one_hand_spear": MaterialSteel,
	"item_mid_warhammer": MaterialSteel,
	"item_mid_morning_star": MaterialSteel,
	"item_mid_mace": MaterialSteel,
	"item_mid_two_hand_sword": MaterialSteel,
	"item_mid_two_hand_axe": MaterialSteel,
	"item_mid_two_hand_spear": MaterialSteel,
	"item_mid_knuckles": MaterialSteel,
	"item_mid_claw": MaterialSteel,
	"item_mid_dual_blades": MaterialSteel,
	"item_mid_bow": MaterialReinforcedWood,
	"item_mid_crossbow": MaterialSteel,
	"item_mid_sling": MaterialReinforcedLeather,
	"item_mid_staff": MaterialRunewood,
	"item_high_one_hand_sword": MaterialStarsteel,
	"item_high_dagger": MaterialStarsteel,
	"item_high_one_hand_axe": MaterialStarsteel,
	"item_high_one_hand_spear": MaterialStarsteel,
	"item_high_warhammer": MaterialStarsteel,
	"item_high_morning_star": MaterialStarsteel,
	"item_high_mace": MaterialStarsteel,
	"item_high_two_hand_sword": MaterialStarsteel,
	"item_high_two_hand_axe": MaterialStarsteel,
	"item_high_two_hand_spear": MaterialStarsteel,
	"item_high_knuckles": MaterialStarsteel,
	"item_high_claw": MaterialStarsteel,
	"item_high_dual_blades": MaterialStarsteel,
	"item_high_bow": MaterialStarwood,
	"item_high_crossbow": MaterialStarsteel,
	"item_high_sling": MaterialStarhide,
	"item_high_staff": MaterialStarwood,
	"item_iron_rim_round_shield": MaterialWood,
	"item_mid_iron_rim_round_shield": MaterialSteel,
	"item_high_iron_rim_round_shield": MaterialStarsteel,
	"item_guard_shield": MaterialIron,
	"item_mid_guard_shield": MaterialSteel,
	"item_high_guard_shield": MaterialStarsteel,
	"item_runed_square_shield": MaterialRunewood,
	"item_mid_runed_square_shield": MaterialSilver,
	"item_high_runed_square_shield": MaterialStarsteel,
	"item_low_cloth_helmet": MaterialCloth,
	"item_low_cloth_chest": MaterialCloth,
	"item_low_cloth_gloves": MaterialCloth,
	"item_low_cloth_legs": MaterialCloth,
	"item_low_cloth_boots": MaterialCloth,
	"item_low_leather_helmet": MaterialLeather,
	"item_low_leather_chest": MaterialLeather,
	"item_low_leather_gloves": MaterialLeather,
	"item_low_leather_legs": MaterialLeather,
	"item_low_leather_boots": MaterialLeather,
	"item_low_heavy_helmet": MaterialIron,
	"item_low_heavy_chest": MaterialIron,
	"item_low_heavy_gloves": MaterialIron,
	"item_low_heavy_legs": MaterialIron,
	"item_low_heavy_boots": MaterialIron,
	"item_garrison_steel_helm": MaterialSteel,
	"item_garrison_steel_cuirass": MaterialSteel,
	"item_garrison_steel_gauntlets": MaterialSteel,
	"item_garrison_steel_greaves": MaterialSteel,
	"item_garrison_steel_boots": MaterialSteel,
	"item_windchaser_cap": MaterialReinforcedLeather,
	"item_windchaser_armor": MaterialReinforcedLeather,
	"item_windchaser_gloves": MaterialReinforcedLeather,
	"item_windchaser_leggings": MaterialReinforcedLeather,
	"item_windchaser_boots": MaterialReinforcedLeather,
	"item_arcane_rune_hood": MaterialRunedCloth,
	"item_arcane_rune_robe": MaterialRunedCloth,
	"item_arcane_rune_gloves": MaterialRunedCloth,
	"item_arcane_rune_trousers": MaterialRunedCloth,
	"item_arcane_rune_boots": MaterialRunedCloth,
	"item_starforged_bastion_helm": MaterialStarsteel,
	"item_starforged_bastion_cuirass": MaterialStarsteel,
	"item_starforged_bastion_gauntlets": MaterialStarsteel,
	"item_starforged_bastion_greaves": MaterialStarsteel,
	"item_starforged_bastion_boots": MaterialStarsteel,
	"item_starshadow_hunter_cap": MaterialStarhide,
	"item_starshadow_hunter_armor": MaterialStarhide,
	"item_starshadow_hunter_gloves": MaterialStarhide,
	"item_starshadow_hunter_leggings": MaterialStarhide,
	"item_starshadow_hunter_boots": MaterialStarhide,
	"item_starlight_hood": MaterialStarweave,
	"item_starlight_robe": MaterialStarweave,
	"item_starlight_gloves": MaterialStarweave,
	"item_starlight_trousers": MaterialStarweave,
	"item_starlight_boots": MaterialStarweave,
}

func applyDefaultProductionMaterials(items []Definition) error {
	if len(items) != len(defaultProductionMaterialByItem) {
		return ErrInvalidCatalog
	}
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		id := strings.TrimSpace(items[i].ItemArchetypeID)
		material, ok := defaultProductionMaterialByItem[id]
		if !ok || !material.Valid() {
			return ErrInvalidCatalog
		}
		if _, duplicate := seen[id]; duplicate {
			return ErrInvalidCatalog
		}
		seen[id] = struct{}{}
		items[i].Material = material
	}
	if len(seen) != len(defaultProductionMaterialByItem) {
		return ErrInvalidCatalog
	}
	return nil
}
