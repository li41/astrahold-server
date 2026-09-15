// Package skilluse defines classless active-skill legality before combat resolution.
package skilluse

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

var (
	ErrInvalidState       = errors.New("skilluse: invalid character skill state")
	ErrPassiveSkill       = errors.New("skilluse: passive skill cannot be actively used")
	ErrSkillNotLearned    = errors.New("skilluse: skill is not learned")
	ErrSkillNotConfigured = errors.New("skilluse: skill is not configured")
	ErrSkillUnavailable   = errors.New("skilluse: skill has no active-use policy")
)

// ValidateActive decides only classless learned/configured legality. Equipment, target,
// range, resources, cooldowns and effects remain separate authoritative checks.
// Universal and support active abilities are fixed abilities and do not consume the
// six configurable combat slots. Configurable melee/ranged/magic skills must be both
// learned and present in the six-slot loadout.
func ValidateActive(id skillcatalog.ID, learned learnedskills.Set, loadout skillloadout.Slots) error {
	if err := validateState(learned, loadout); err != nil {
		return err
	}
	definition, ok := skillcatalog.Lookup(id)
	if !ok {
		return fmt.Errorf("%w: %q", skillcatalog.ErrUnknownSkill, id)
	}
	if definition.Activation != skillcatalog.ActivationActive {
		return fmt.Errorf("%w: %q", ErrPassiveSkill, id)
	}
	if definition.CombatLoadoutEligible {
		if !learned.Contains(id) {
			return fmt.Errorf("%w: %q", ErrSkillNotLearned, id)
		}
		if !contains(loadout, id) {
			return fmt.Errorf("%w: %q", ErrSkillNotConfigured, id)
		}
		return nil
	}
	switch definition.Category {
	case skillcatalog.CategoryUniversal, skillcatalog.CategorySupport:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrSkillUnavailable, id)
	}
}

func validateState(learned learnedskills.Set, loadout skillloadout.Slots) error {
	if err := learned.Validate(); err != nil {
		return fmt.Errorf("%w: learned: %v", ErrInvalidState, err)
	}
	if err := loadout.Validate(); err != nil {
		return fmt.Errorf("%w: loadout: %v", ErrInvalidState, err)
	}
	for _, id := range loadout.IDs() {
		if !learned.Contains(id) {
			return fmt.Errorf("%w: configured skill %q is not learned", ErrInvalidState, id)
		}
	}
	return nil
}

func contains(loadout skillloadout.Slots, id skillcatalog.ID) bool {
	for _, configured := range loadout.IDs() {
		if configured == id {
			return true
		}
	}
	return false
}
