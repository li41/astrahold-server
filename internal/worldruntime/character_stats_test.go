package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
)

func TestEffectivePrimaryStatsFromLearnedAppliesAuthoredPassives(t *testing.T) {
	learned, err := learnedskills.NewSet([]skillcatalog.ID{
		skillcatalog.StrongPhysique,
		skillcatalog.ClearMeridians,
		skillcatalog.SpiritConcentration,
		skillcatalog.Haste,
		skillcatalog.HeavyStrike,
	})
	if err != nil { t.Fatal(err) }
	got, err := effectivePrimaryStatsFromLearned(characterstats.Primary{Strength: 11, Agility: 13}, learned)
	if err != nil { t.Fatal(err) }
	want := characterstats.Primary{Strength: 16, Agility: 18}
	if got != want { t.Fatalf("got=%#v want=%#v", got, want) }
}

func TestEffectivePrimaryStatsFromLearnedDoesNotInventUnauthoredBonuses(t *testing.T) {
	learned, err := learnedskills.NewSet([]skillcatalog.ID{
		skillcatalog.SpiritConcentration,
		skillcatalog.Haste,
	})
	if err != nil { t.Fatal(err) }
	base := characterstats.Primary{Strength: 3, Agility: 4}
	got, err := effectivePrimaryStatsFromLearned(base, learned)
	if err != nil { t.Fatal(err) }
	if got != base { t.Fatalf("got=%#v want=%#v", got, base) }
}