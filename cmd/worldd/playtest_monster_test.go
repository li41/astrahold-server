package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/loot"
)

func TestPlaytestMonsterLootCatalogUsesCommonPeltChance(t *testing.T) {
	catalog := newPlaytestMonsterLootCatalog()
	drops, ok := catalog.DropsFor(playtestMonsterArchetypeID)
	if !ok || len(drops) != 1 {
		t.Fatalf("wolf loot drops=%#v ok=%v", drops, ok)
	}
	drop := drops[0]
	if drop.ItemArchetypeID != playtestMonsterDropArchetypeID {
		t.Fatalf("wolf drop archetype=%q want=%q", drop.ItemArchetypeID, playtestMonsterDropArchetypeID)
	}
	if drop.ChanceBasisPoints != playtestMonsterDropChanceBasisPoints {
		t.Fatalf("wolf pelt chance=%d want=%d", drop.ChanceBasisPoints, playtestMonsterDropChanceBasisPoints)
	}
	if drop.ChanceBasisPoints == 0 || drop.ChanceBasisPoints >= loot.ChanceBasisPointsScale {
		t.Fatalf("wolf pelt chance=%d should be common but non-guaranteed", drop.ChanceBasisPoints)
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
