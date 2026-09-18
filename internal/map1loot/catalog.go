package map1loot

import (
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/monstercatalog"
)

const (
	ItemGoldCoin = "item_gold_coin"
)

func Default() (*loot.Catalog, error) {
	return loot.New(loot.Definition{
		Revision: "map1-loot-v1",
		Tables: []loot.Table{
			{SourceArchetypeID: monstercatalog.MonsterGrayWolf, Drops: grayWolfDrops()},
			{SourceArchetypeID: monstercatalog.MonsterWildBoar, Drops: wildBoarDrops()},
			{SourceArchetypeID: monstercatalog.MonsterWitherwillDeserter, Drops: witherwillDeserterDrops()},
			{SourceArchetypeID: monstercatalog.MonsterWitherwillEnforcer, Drops: witherwillEnforcerDrops()},
			{SourceArchetypeID: monstercatalog.MonsterWitherwillCaptain, Drops: witherwillCaptainDrops()},
			{SourceArchetypeID: monstercatalog.MonsterRedsoilWorkerAnt, Drops: redsoilWorkerAntDrops()},
			{SourceArchetypeID: monstercatalog.MonsterRedsoilSoldierAnt, Drops: redsoilSoldierAntDrops()},
			{SourceArchetypeID: monstercatalog.MonsterRedsoilGuardAnt, Drops: redsoilGuardAntDrops()},
			{SourceArchetypeID: monstercatalog.MonsterRedsoilQueen, Drops: redsoilQueenDrops()},
			{SourceArchetypeID: monstercatalog.MonsterShoreCrab, Drops: shoreCrabDrops()},
		},
	})
}

func stack(id string, chance uint16, min, max uint32) loot.Drop {
	return loot.Drop{
		Kind:              loot.DropKindStack,
		ItemArchetypeID:   id,
		ChanceBasisPoints: chance,
		QuantityMin:       min,
		QuantityMax:       max,
	}
}

func one(id string, chance uint16) loot.Drop {
	return stack(id, chance, 1, 1)
}

func instance(id string, chance uint16) loot.Drop {
	return loot.Drop{
		Kind:              loot.DropKindEquipmentInstance,
		ItemArchetypeID:   id,
		ChanceBasisPoints: chance,
		QuantityMin:       1,
		QuantityMax:       1,
	}
}

func gold(min, max uint32) loot.Drop {
	return stack(ItemGoldCoin, loot.ChanceBasisPointsScale, min, max)
}

func grayWolfDrops() []loot.Drop {
	return []loot.Drop{
		gold(1, 3),
		one("item_gray_wolf_pelt", 4_500),
		one("item_minor_healing_potion", 400),
		one("item_minor_speed_potion", 100),
		one("item_low_leather_helmet", 100),
		one("item_low_leather_chest", 100),
		one("item_low_leather_gloves", 100),
		one("item_low_leather_legs", 100),
		one("item_low_leather_boots", 100),
	}
}

func wildBoarDrops() []loot.Drop {
	return []loot.Drop{
		gold(2, 4),
		one("item_minor_healing_potion", 500),
		one("item_low_heavy_helmet", 100),
		one("item_low_heavy_chest", 100),
		one("item_low_heavy_gloves", 100),
		one("item_low_heavy_legs", 100),
		one("item_low_heavy_boots", 100),
		one("item_militia_iron_spear", 100),
		one("item_militia_battle_axe", 100),
		one("item_iron_war_mace", 100),
	}
}

func witherwillDeserterDrops() []loot.Drop {
	return []loot.Drop{
		gold(4, 8),
		one("item_minor_healing_potion", 800),
		one("item_minor_mana_potion", 500),
		one("item_minor_speed_potion", 300),
		stack("item_low_arrow", 1_200, 4, 8),
		one("item_militia_iron_sword", 100),
		one("item_light_guard_sword", 100),
		one("item_iron_dagger", 100),
		one("item_militia_battle_axe", 100),
		one("item_militia_iron_spear", 100),
		one("item_hunter_shortbow", 100),
		one("item_hunter_light_crossbow", 100),
		one("item_low_leather_helmet", 100),
		one("item_low_leather_chest", 100),
		one("item_low_leather_legs", 100),
		one("item_low_heavy_helmet", 100),
		one("item_iron_rim_round_shield", 100),
	}
}

func witherwillEnforcerDrops() []loot.Drop {
	return []loot.Drop{
		gold(6, 12),
		one("item_minor_healing_potion", 1_000),
		one("item_minor_speed_potion", 400),
		stack("item_low_arrow", 1_200, 4, 8),
		one("item_gladiator_iron_sword", 100),
		one("item_iron_warhammer", 100),
		one("item_iron_morning_star", 100),
		one("item_iron_war_mace", 100),
		one("item_two_hand_iron_sword", 100),
		one("item_two_hand_battle_axe", 100),
		one("item_long_iron_spear", 100),
		one("item_militia_dual_blades", 100),
		one("item_low_heavy_helmet", 100),
		one("item_low_heavy_chest", 100),
		one("item_low_heavy_gloves", 100),
		one("item_low_heavy_legs", 100),
		one("item_low_heavy_boots", 100),
		one("item_iron_rim_round_shield", 100),
		one("item_guard_shield", 100),
		one("item_runed_square_shield", 100),
	}
}

func witherwillCaptainDrops() []loot.Drop {
	return []loot.Drop{
		gold(20, 35),
		one("item_minor_healing_potion", 2_000),
		one("item_minor_mana_potion", 1_200),
		one("item_minor_speed_potion", 1_200),
		stack("item_low_arrow", 2_500, 8, 16),

		one("item_gladiator_iron_sword", 250),
		one("item_iron_warhammer", 250),
		one("item_iron_morning_star", 250),
		one("item_two_hand_iron_sword", 250),
		one("item_two_hand_battle_axe", 250),
		one("item_long_iron_spear", 250),
		one("item_guard_shield", 250),
		one("item_low_heavy_chest", 250),
		one("item_low_leather_chest", 250),
		one("item_low_cloth_chest", 250),

		instance("item_mid_one_hand_sword", 100),
		instance("item_mid_dagger", 100),
		instance("item_mid_one_hand_axe", 100),
		instance("item_mid_one_hand_spear", 100),
		instance("item_mid_warhammer", 100),
		instance("item_mid_two_hand_sword", 100),
		instance("item_mid_bow", 100),
		instance("item_mid_crossbow", 100),

		instance("item_garrison_steel_helm", 120),
		instance("item_garrison_steel_cuirass", 120),
		instance("item_garrison_steel_gauntlets", 120),
		instance("item_garrison_steel_greaves", 120),
		instance("item_garrison_steel_boots", 120),

		one("item_astrahold_weapon_enhancement_scroll", 200),
		one("item_astrahold_armor_enhancement_scroll", 200),
	}
}

func redsoilWorkerAntDrops() []loot.Drop {
	return []loot.Drop{
		gold(1, 2),
		one("item_minor_healing_potion", 300),
		one("item_low_cloth_gloves", 100),
		one("item_low_cloth_boots", 100),
	}
}

func redsoilSoldierAntDrops() []loot.Drop {
	return []loot.Drop{
		gold(2, 5),
		one("item_minor_healing_potion", 500),
		one("item_minor_speed_potion", 100),
		one("item_low_leather_helmet", 50),
		one("item_low_leather_gloves", 100),
		one("item_low_leather_boots", 100),
		one("item_low_heavy_helmet", 50),
		one("item_low_heavy_gloves", 50),
		one("item_low_heavy_boots", 50),
		one("item_militia_iron_spear", 50),
		one("item_iron_war_mace", 50),
		one("item_iron_rim_round_shield", 50),
		one("item_guard_shield", 50),
	}
}

func redsoilGuardAntDrops() []loot.Drop {
	return []loot.Drop{
		gold(5, 9),
		one("item_minor_healing_potion", 800),
		one("item_minor_speed_potion", 300),
		one("item_low_leather_chest", 100),
		one("item_low_leather_legs", 100),
		one("item_low_heavy_chest", 100),
		one("item_low_heavy_legs", 100),
		one("item_guard_shield", 100),
		one("item_runed_square_shield", 100),
	}
}

func redsoilQueenDrops() []loot.Drop {
	drops := []loot.Drop{
		gold(35, 60),
		one("item_minor_healing_potion", 3_500),
		one("item_minor_mana_potion", 2_000),
		one("item_minor_speed_potion", 800),

		one("item_low_cloth_helmet", 250),
		one("item_low_cloth_chest", 250),
		one("item_low_leather_helmet", 250),
		one("item_low_leather_chest", 250),
		one("item_low_heavy_helmet", 250),
		one("item_low_heavy_chest", 250),
		one("item_guard_shield", 250),
		one("item_runed_square_shield", 250),
		one("item_two_hand_iron_sword", 250),
		one("item_two_hand_battle_axe", 250),
		one("item_hunter_shortbow", 250),
		one("item_apprentice_wood_staff", 250),
	}

	for _, id := range []string{
		"item_mid_one_hand_sword",
		"item_mid_dagger",
		"item_mid_one_hand_axe",
		"item_mid_one_hand_spear",
		"item_mid_warhammer",
		"item_mid_morning_star",
		"item_mid_mace",
		"item_mid_two_hand_sword",
		"item_mid_two_hand_axe",
		"item_mid_two_hand_spear",
		"item_mid_knuckles",
		"item_mid_claw",
		"item_mid_dual_blades",
		"item_mid_bow",
		"item_mid_crossbow",
		"item_mid_sling",
		"item_mid_staff",
	} {
		drops = append(drops, instance(id, 70))
	}

	for _, id := range []string{
		"item_garrison_steel_helm",
		"item_garrison_steel_cuirass",
		"item_garrison_steel_gauntlets",
		"item_garrison_steel_greaves",
		"item_garrison_steel_boots",
		"item_windchaser_cap",
		"item_windchaser_armor",
		"item_windchaser_gloves",
		"item_windchaser_leggings",
		"item_windchaser_boots",
		"item_arcane_rune_hood",
		"item_arcane_rune_robe",
		"item_arcane_rune_gloves",
		"item_arcane_rune_trousers",
		"item_arcane_rune_boots",
	} {
		drops = append(drops, instance(id, 100))
	}

	for _, id := range []string{
		"item_mid_iron_rim_round_shield",
		"item_mid_guard_shield",
		"item_mid_runed_square_shield",
	} {
		drops = append(drops, instance(id, 167))
	}

	drops = append(drops,
		one("item_astrahold_weapon_enhancement_scroll", 300),
		one("item_astrahold_armor_enhancement_scroll", 300),
	)
	return drops
}

func shoreCrabDrops() []loot.Drop {
	return []loot.Drop{
		gold(2, 4),
		one("item_minor_healing_potion", 500),
		one("item_iron_rim_round_shield", 150),
		one("item_low_heavy_gloves", 100),
		one("item_low_heavy_boots", 100),
		one("item_low_heavy_helmet", 50),
	}
}
