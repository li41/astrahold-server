package skillcatalog

import (
	"errors"
	"testing"
)

func TestCatalogLocksClasslessSkillSetV1(t *testing.T) {
	expected := map[ID]struct {
		category   Category
		activation Activation
		combat     bool
	}{
		RandomTeleport:      {CategoryUniversal, ActivationActive, false},
		Sunlight:            {CategoryUniversal, ActivationActive, false},
		Guard:               {CategoryUniversal, ActivationActive, false},
		Heal:                {CategoryUniversal, ActivationActive, false},
		HeavyStrike:         {CategoryMelee, ActivationActive, true},
		Flurry:              {CategoryMelee, ActivationActive, true},
		Cleave:              {CategoryMelee, ActivationActive, true},
		Execute:             {CategoryMelee, ActivationActive, true},
		PiercingShot:        {CategoryRanged, ActivationActive, true},
		RapidShot:           {CategoryRanged, ActivationActive, true},
		Volley:              {CategoryRanged, ActivationActive, true},
		PinningShot:         {CategoryRanged, ActivationActive, true},
		FireBolt:            {CategoryMagic, ActivationActive, true},
		FrostBurst:          {CategoryMagic, ActivationActive, true},
		Meteor:              {CategoryMagic, ActivationActive, true},
		ArcaneLance:         {CategoryMagic, ActivationActive, true},
		StrongPhysique:      {CategorySupport, ActivationPassive, false},
		ClearMeridians:      {CategorySupport, ActivationPassive, false},
		SpiritConcentration: {CategorySupport, ActivationPassive, false},
		Haste:               {CategorySupport, ActivationActive, false},
	}

	all := All()
	if len(all) != len(expected) {
		t.Fatalf("skill count = %d, want %d", len(all), len(expected))
	}

	categoryCounts := map[Category]int{}
	seen := make(map[ID]struct{}, len(all))
	combatCount := 0
	fixedCount := 0
	for _, definition := range all {
		want, ok := expected[definition.ID]
		if !ok {
			t.Fatalf("unexpected skill %q", definition.ID)
		}
		if _, duplicate := seen[definition.ID]; duplicate {
			t.Fatalf("duplicate skill %q", definition.ID)
		}
		seen[definition.ID] = struct{}{}
		if definition.Category != want.category || definition.Activation != want.activation || definition.CombatLoadoutEligible != want.combat {
			t.Fatalf("%q = %#v, want category=%q activation=%q combat=%v", definition.ID, definition, want.category, want.activation, want.combat)
		}
		categoryCounts[definition.Category]++
		if definition.CombatLoadoutEligible {
			combatCount++
		} else {
			fixedCount++
		}
	}

	for _, category := range []Category{CategoryUniversal, CategoryMelee, CategoryRanged, CategoryMagic, CategorySupport} {
		if got := categoryCounts[category]; got != 4 {
			t.Fatalf("category %q count = %d, want 4", category, got)
		}
	}
	if combatCount != 12 {
		t.Fatalf("combat-loadout eligible count = %d, want 12", combatCount)
	}
	if fixedCount != 8 {
		t.Fatalf("fixed universal/support count = %d, want 8", fixedCount)
	}
}

func TestValidateCombatLoadoutAcceptsSixMixedCombatSkills(t *testing.T) {
	loadout := []ID{HeavyStrike, Flurry, PiercingShot, Volley, FireBolt, Meteor}
	if err := ValidateCombatLoadout(loadout); err != nil {
		t.Fatalf("ValidateCombatLoadout() error = %v", err)
	}
}

func TestValidateCombatLoadoutRejectsSeventhSkill(t *testing.T) {
	loadout := []ID{HeavyStrike, Flurry, Cleave, PiercingShot, RapidShot, FireBolt, Meteor}
	if err := ValidateCombatLoadout(loadout); !errors.Is(err, ErrCombatLoadoutTooLarge) {
		t.Fatalf("error = %v, want ErrCombatLoadoutTooLarge", err)
	}
}

func TestValidateCombatLoadoutRejectsFixedUniversalAndSupportSkills(t *testing.T) {
	for _, id := range []ID{RandomTeleport, Sunlight, Guard, Heal, StrongPhysique, ClearMeridians, SpiritConcentration, Haste} {
		if err := ValidateCombatLoadout([]ID{id}); !errors.Is(err, ErrNotCombatLoadoutSkill) {
			t.Fatalf("skill %q error = %v, want ErrNotCombatLoadoutSkill", id, err)
		}
	}
}

func TestValidateCombatLoadoutRejectsUnknownAndDuplicateSkills(t *testing.T) {
	if err := ValidateCombatLoadout([]ID{"unknown-skill"}); !errors.Is(err, ErrUnknownSkill) {
		t.Fatalf("unknown error = %v, want ErrUnknownSkill", err)
	}
	if err := ValidateCombatLoadout([]ID{HeavyStrike, HeavyStrike}); !errors.Is(err, ErrDuplicateSkill) {
		t.Fatalf("duplicate error = %v, want ErrDuplicateSkill", err)
	}
}

func TestAllReturnsCopy(t *testing.T) {
	all := All()
	all[0].ID = "mutated"
	if _, ok := Lookup(RandomTeleport); !ok {
		t.Fatal("mutating All result changed canonical catalog")
	}
}
