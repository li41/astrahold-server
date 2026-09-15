package skilluse

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

func TestValidateActiveFixedAbilitiesDoNotRequireCombatSlots(t *testing.T) {
	for _, id := range []skillcatalog.ID{
		skillcatalog.RandomTeleport,
		skillcatalog.Sunlight,
		skillcatalog.Guard,
		skillcatalog.Heal,
		skillcatalog.Haste,
	} {
		if err := ValidateActive(id, learnedskills.Set{}, skillloadout.Slots{}); err != nil {
			t.Fatalf("fixed active %q err=%v", id, err)
		}
	}
}

func TestValidateActiveConfigurableSkillRequiresLearnedAndConfigured(t *testing.T) {
	learned := mustLearned(t, skillcatalog.HeavyStrike)
	configured := mustLoadout(t, skillcatalog.HeavyStrike)
	if err := ValidateActive(skillcatalog.HeavyStrike, learned, configured); err != nil {
		t.Fatalf("learned configured err=%v", err)
	}
	if err := ValidateActive(skillcatalog.HeavyStrike, learned, skillloadout.Slots{}); !errors.Is(err, ErrSkillNotConfigured) {
		t.Fatalf("not configured err=%v", err)
	}
	if err := ValidateActive(skillcatalog.HeavyStrike, learnedskills.Set{}, skillloadout.Slots{}); !errors.Is(err, ErrSkillNotLearned) {
		t.Fatalf("not learned err=%v", err)
	}
}

func TestValidateActiveRejectsPassiveAndUnknownSkills(t *testing.T) {
	learned := mustLearned(t, skillcatalog.StrongPhysique)
	if err := ValidateActive(skillcatalog.StrongPhysique, learned, skillloadout.Slots{}); !errors.Is(err, ErrPassiveSkill) {
		t.Fatalf("passive err=%v", err)
	}
	if err := ValidateActive(skillcatalog.ID("missing-skill"), learnedskills.Set{}, skillloadout.Slots{}); !errors.Is(err, skillcatalog.ErrUnknownSkill) {
		t.Fatalf("unknown err=%v", err)
	}
}

func TestValidateActiveFailsClosedOnInconsistentCharacterState(t *testing.T) {
	loadout := mustLoadout(t, skillcatalog.FireBolt)
	if err := ValidateActive(skillcatalog.Guard, learnedskills.Set{}, loadout); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("inconsistent state err=%v", err)
	}
}

func TestValidateActiveCoversFormalV1Catalog(t *testing.T) {
	for _, definition := range skillcatalog.All() {
		if definition.Activation != skillcatalog.ActivationActive {
			continue
		}
		var learned learnedskills.Set
		var loadout skillloadout.Slots
		if definition.CombatLoadoutEligible {
			learned = mustLearned(t, definition.ID)
			loadout = mustLoadout(t, definition.ID)
		}
		if err := ValidateActive(definition.ID, learned, loadout); err != nil {
			t.Fatalf("active skill %q err=%v", definition.ID, err)
		}
	}
}

func mustLearned(t *testing.T, ids ...skillcatalog.ID) learnedskills.Set {
	t.Helper()
	set, err := learnedskills.NewSet(ids)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func mustLoadout(t *testing.T, ids ...skillcatalog.ID) skillloadout.Slots {
	t.Helper()
	slots, err := skillloadout.NewSlots(ids)
	if err != nil {
		t.Fatal(err)
	}
	return slots
}
