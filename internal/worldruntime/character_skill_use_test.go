package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skilluse"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCharacterSkillRuntimeValidateActiveUseReadsAuthoritativeState(t *testing.T) {
	var state characterSkillRuntime
	entityID := world.EntityID(41)

	if err := state.validateActiveUse(entityID, skillcatalog.Guard); err != nil {
		t.Fatalf("fixed guard err=%v", err)
	}
	if err := state.learned.SetLearned(entityID, []skillcatalog.ID{skillcatalog.HeavyStrike}); err != nil {
		t.Fatal(err)
	}
	if err := state.validateActiveUse(entityID, skillcatalog.HeavyStrike); !errors.Is(err, skilluse.ErrSkillNotConfigured) {
		t.Fatalf("learned but not configured err=%v", err)
	}
	if err := state.loadout.SetCombat(entityID, []skillcatalog.ID{skillcatalog.HeavyStrike}, func(id skillcatalog.ID) bool {
		return state.learned.Contains(entityID, id)
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.validateActiveUse(entityID, skillcatalog.HeavyStrike); err != nil {
		t.Fatalf("learned configured err=%v", err)
	}
}

func TestCharacterSkillRuntimeValidateActiveUseRejectsInvalidEntity(t *testing.T) {
	var state characterSkillRuntime
	if err := state.validateActiveUse(0, skillcatalog.Guard); !errors.Is(err, ErrCharacterSkillStateInvalid) {
		t.Fatalf("zero entity err=%v", err)
	}
}
