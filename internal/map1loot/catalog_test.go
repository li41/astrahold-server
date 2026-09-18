package map1loot

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/monstercatalog"
)

func TestDefaultMap1LootCoversAllFormalMonsters(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range monstercatalog.Map1().IDs() {
		drops, ok := catalog.DropsFor(id)
		if !ok || len(drops) == 0 {
			t.Fatalf("missing loot table for %s", id)
		}
		gold, ok := findDrop(drops, ItemGoldCoin)
		if !ok {
			t.Fatalf("%s missing gold", id)
		}
		if gold.Kind != loot.DropKindStack || gold.ChanceBasisPoints != loot.ChanceBasisPointsScale || gold.QuantityMin == 0 || gold.QuantityMax < gold.QuantityMin {
			t.Fatalf("%s invalid gold drop: %+v", id, gold)
		}
	}
}

func TestDefaultMap1LootRepresentativeExactRows(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		source string
		item   string
		kind   loot.DropKind
		chance uint16
		min    uint32
		max    uint32
	}{
		{monstercatalog.MonsterGrayWolf, ItemGoldCoin, loot.DropKindStack, 10_000, 1, 3},
		{monstercatalog.MonsterGrayWolf, "item_gray_wolf_pelt", loot.DropKindStack, 4_500, 1, 1},
		{monstercatalog.MonsterWitherwillDeserter, "item_low_arrow", loot.DropKindStack, 1_200, 4, 8},
		{monstercatalog.MonsterWitherwillCaptain, "item_mid_one_hand_sword", loot.DropKindEquipmentInstance, 100, 1, 1},
		{monstercatalog.MonsterWitherwillCaptain, "item_garrison_steel_cuirass", loot.DropKindEquipmentInstance, 120, 1, 1},
		{monstercatalog.MonsterRedsoilQueen, ItemGoldCoin, loot.DropKindStack, 10_000, 35, 60},
		{monstercatalog.MonsterRedsoilQueen, "item_mid_staff", loot.DropKindEquipmentInstance, 70, 1, 1},
		{monstercatalog.MonsterRedsoilQueen, "item_mid_guard_shield", loot.DropKindEquipmentInstance, 167, 1, 1},
		{monstercatalog.MonsterShoreCrab, "item_iron_rim_round_shield", loot.DropKindStack, 150, 1, 1},
	}
	for _, tc := range cases {
		drops, _ := catalog.DropsFor(tc.source)
		got, ok := findDrop(drops, tc.item)
		if !ok {
			t.Fatalf("%s missing %s", tc.source, tc.item)
		}
		if got.Kind != tc.kind || got.ChanceBasisPoints != tc.chance || got.QuantityMin != tc.min || got.QuantityMax != tc.max {
			t.Fatalf("%s/%s got=%+v", tc.source, tc.item, got)
		}
	}
}

func TestDefaultMap1UniqueDropsResolveToMidTierEquipment(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	equipment, err := equipmentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range monstercatalog.Map1().IDs() {
		drops, _ := catalog.DropsFor(source)
		for _, drop := range drops {
			if drop.Kind != loot.DropKindEquipmentInstance {
				continue
			}
			definition, ok := equipment.Resolve(drop.ItemArchetypeID)
			if !ok {
				t.Fatalf("%s unique drop missing equipment definition: %s", source, drop.ItemArchetypeID)
			}
			if definition.Tier != equipmentcatalog.TierMid {
				t.Fatalf("%s unique drop %s tier=%s want mid", source, drop.ItemArchetypeID, definition.Tier)
			}
		}
	}
}

func TestRedsoilGuardHasNoTierMidDrop(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	drops, _ := catalog.DropsFor(monstercatalog.MonsterRedsoilGuardAnt)
	for _, drop := range drops {
		if drop.Kind == loot.DropKindEquipmentInstance {
			t.Fatalf("guard ant must not drop TierMid instance: %+v", drop)
		}
	}
}

func findDrop(drops []loot.Drop, item string) (loot.Drop, bool) {
	for _, drop := range drops {
		if drop.ItemArchetypeID == item {
			return drop, true
		}
	}
	return loot.Drop{}, false
}
