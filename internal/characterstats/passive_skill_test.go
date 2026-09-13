package characterstats

import (
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
)

func TestPrimaryBonusForSkillAuthoredPassives(t *testing.T) {
	tests := []struct {
		name string
		skillID skillcatalog.ID
		want AdditiveBonus
	}{
		{name: "strong physique", skillID: skillcatalog.StrongPhysique, want: AdditiveBonus{Strength: 5}},
		{name: "clear meridians", skillID: skillcatalog.ClearMeridians, want: AdditiveBonus{Agility: 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PrimaryBonusForSkill(tt.skillID)
			if !ok {
				t.Fatal("PrimaryBonusForSkill() ok = false, want true")
			}
			if got != tt.want {
				t.Fatalf("PrimaryBonusForSkill() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPrimaryBonusForSkillLeavesUndecidedEffectsUnauthored(t *testing.T) {
	for _, skillID := range []skillcatalog.ID{
		skillcatalog.SpiritConcentration,
		skillcatalog.Haste,
		skillcatalog.HeavyStrike,
	} {
		if got, ok := PrimaryBonusForSkill(skillID); ok || got != (AdditiveBonus{}) {
			t.Fatalf("PrimaryBonusForSkill(%q) = (%+v, %v), want zero false", skillID, got, ok)
		}
	}
}
