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
