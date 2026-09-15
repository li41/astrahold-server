package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/equipmentstats"
)

func TestPrimaryBonusFromEquipmentModifiersMapsAllPrimaryStats(t *testing.T) {
	got := primaryBonusFromEquipmentModifiers(equipmentstats.Modifiers{
		Strength: 1,
		Dexterity: 2,
		Constitution: 3,
		Intelligence: 4,
		Spirit: 5,
		Charisma: 6,
		PhysicalHit: 99,
		MagicPower: 99,
	})
	want := characterstats.AdditiveBonus{
		Strength: 1,
		Agility: 2,
		Constitution: 3,
		Intelligence: 4,
		Spirit: 5,
		Charisma: 6,
	}
	if got != want {
		t.Fatalf("equipment primary bonus=%#v want=%#v", got, want)
	}
}
