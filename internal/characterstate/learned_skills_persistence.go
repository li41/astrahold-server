package characterstate

import (
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

func learnedSkillsToWire(set learnedskills.Set) []string {
	ids := set.IDs()
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func learnedSkillsFromWire(raw []string) (learnedskills.Set, error) {
	ids := make([]skillcatalog.ID, len(raw))
	for i, id := range raw {
		ids[i] = skillcatalog.ID(id)
	}
	set, err := learnedskills.NewSet(ids)
	if err != nil {
		return learnedskills.Set{}, ErrInvalidSnapshot
	}
	return set, nil
}

// learnedSkillsFromCombatLoadout is the only legacy inference used while crossing the
// v7/v6 loadout-only durable boundary. A configured combat skill is direct evidence that
// the character already possessed that skill; no other skill is granted by migration.
func learnedSkillsFromCombatLoadout(loadout skillloadout.Slots) (learnedskills.Set, error) {
	set, err := learnedskills.NewSet(loadout.IDs())
	if err != nil {
		return learnedskills.Set{}, ErrInvalidSnapshot
	}
	return set, nil
}

func validateLearnedSkills(set learnedskills.Set) error {
	if err := set.Validate(); err != nil {
		return ErrInvalidSnapshot
	}
	return nil
}

func validateCombatLoadoutLearned(loadout skillloadout.Slots, learned learnedskills.Set) error {
	for _, id := range loadout.IDs() {
		if !learned.Contains(id) {
			return ErrInvalidSnapshot
		}
	}
	return nil
}
