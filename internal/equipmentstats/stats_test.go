package equipmentstats

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestAggregateEquippedAffixes(t *testing.T) {
	weapon := iteminstance.Instance{
		ID:              "weapon-1",
		ItemArchetypeID: "item_mid_blade",
		Affixes: []equipmentaffix.Affix{
			{ID: equipmentaffix.AffixStrength, Strength: 2, Value: 2},
			{ID: equipmentaffix.AffixCriticalRating, Strength: 1, Value: 1},
		},
	}
	shield := iteminstance.Instance{
		ID:              "shield-1",
		ItemArchetypeID: "item_high_shield",
		Affixes: []equipmentaffix.Affix{
			{ID: equipmentaffix.AffixStrength, Strength: 1, Value: 1},
			{ID: equipmentaffix.AffixMaxHP, Strength: 5, Value: 100},
		},
	}

	got, err := Aggregate(weapon, shield)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Strength != 3 || got.CriticalRating != 1 || got.MaxHP != 100 {
		t.Fatalf("modifiers=%#v", got)
	}
}

func TestAggregateIgnoresNoUnknownGameplayEffect(t *testing.T) {
	got, err := Aggregate(iteminstance.Instance{})
	if err != nil {
		t.Fatalf("Aggregate empty: %v", err)
	}
	if got != (Modifiers{}) {
		t.Fatalf("empty modifiers=%#v", got)
	}
}
