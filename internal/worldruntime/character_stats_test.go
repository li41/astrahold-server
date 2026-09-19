package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/equipmentstats"
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

func TestDerivedCharacterMaxVitalsIncludesEquipmentPrimaryAndFlatBonuses(t *testing.T) {
	base := characterstats.Primary{
		Strength: 10, Agility: 10, Constitution: 16,
		Intelligence: 10, Spirit: 14, Charisma: 10,
	}
	modifiers := equipmentstats.Modifiers{
		Constitution: 2,
		Spirit:       2,
		MaxHP:        40,
		MaxMP:        20,
	}
	maxHP, maxMP, err := derivedCharacterMaxVitals(20, base, learnedskills.Set{}, modifiers)
	if err != nil {
		t.Fatal(err)
	}
	if maxHP != 324 || maxMP != 104 {
		t.Fatalf("max hp/mp=%d/%d want=324/104", maxHP, maxMP)
	}
}
