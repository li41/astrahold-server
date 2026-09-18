package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/map1loot"
)

func TestPlaytestMonsterLootCatalogUsesFormalGrayWolfTable(t *testing.T) {
	catalog := newPlaytestMonsterLootCatalog()
	drops, ok := catalog.DropsFor(playtestMonsterArchetypeID)
	if !ok || len(drops) == 0 {
		t.Fatalf("wolf loot drops=%#v ok=%v", drops, ok)
	}
	var pelt, gold loot.Drop
	var sawPelt, sawGold bool
	for _, drop := range drops {
		switch drop.ItemArchetypeID {
		case playtestMonsterDropArchetypeID:
			pelt, sawPelt = drop, true
		case map1loot.ItemGoldCoin:
			gold, sawGold = drop, true
		}
	}
	if !sawPelt || pelt.ChanceBasisPoints != playtestMonsterDropChanceBasisPoints || pelt.QuantityMin != 1 || pelt.QuantityMax != 1 {
		t.Fatalf("formal wolf pelt drop=%#v found=%v", pelt, sawPelt)
	}
	if pelt.ChanceBasisPoints == 0 || pelt.ChanceBasisPoints >= loot.ChanceBasisPointsScale {
		t.Fatalf("wolf pelt chance=%d should be common but non-guaranteed", pelt.ChanceBasisPoints)
	}
	if !sawGold || gold.ChanceBasisPoints != loot.ChanceBasisPointsScale || gold.QuantityMin != 1 || gold.QuantityMax != 3 {
		t.Fatalf("formal wolf gold drop=%#v found=%v", gold, sawGold)
	}
}

func TestPlaytestMonsterAIUsesSmallAuthoredIdlePatrol(t *testing.T) {
	config := newPlaytestMonsterAIConfig()
	if len(config.IdlePatrol) != 4 {
		t.Fatalf("idle patrol points=%d want=4", len(config.IdlePatrol))
	}
	if config.PatrolTolerance != playtestMonsterPatrolToleranceMeters || config.PatrolTolerance <= 0 {
		t.Fatalf("patrol tolerance=%v want=%v", config.PatrolTolerance, playtestMonsterPatrolToleranceMeters)
	}
	home := playtestMonsterHome()
	leashSq := config.LeashRange * config.LeashRange
	for index, point := range config.IdlePatrol {
		if point.Layer != home.Layer {
			t.Fatalf("patrol[%d] layer=%d want=%d", index, point.Layer, home.Layer)
		}
		if point.DistanceXZSquared(home) > leashSq {
			t.Fatalf("patrol[%d]=%#v outside leash %.2fm", index, point, config.LeashRange)
		}
	}
	last := config.IdlePatrol[len(config.IdlePatrol)-1]
	if last != home {
		t.Fatalf("idle route does not close at home: last=%#v home=%#v", last, home)
	}
}
