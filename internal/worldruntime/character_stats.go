package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/world"
)

// effectivePrimaryStatsFromLearned composes durable base stats with the effects of learned
// passive skills. Passive bonuses remain derived truth and are never duplicated in persistence.
func effectivePrimaryStatsFromLearned(base characterstats.Primary, learned learnedskills.Set) (characterstats.Primary, error) {
	bonus := characterstats.AdditiveBonus{}
	for _, skillID := range learned.IDs() {
		skillBonus, ok := characterstats.PrimaryBonusForSkill(skillID)
		if !ok {
			continue
		}
		combined, err := characterstats.CombineAdditive(bonus, skillBonus)
		if err != nil {
			return characterstats.Primary{}, err
		}
		bonus = combined
	}
	return characterstats.Effective(base, bonus)
}

// characterEffectivePrimaryStats is the world-owner read seam for future combat scaling.
// It reads the existing authoritative character and learned-skill stores; it does not create a
// second mutable stat authority or infer any unapproved allocation/scaling rules.
func (r *Runtime) characterEffectivePrimaryStats(entityID world.EntityID) (characterstats.Primary, error) {
	state, ok := r.characters.State(entityID)
	if !ok {
		return characterstats.Primary{}, character.ErrCharacterNotFound
	}
	learned, _, err := r.characterSkills.capture(entityID)
	if err != nil {
		return characterstats.Primary{}, err
	}
	return effectivePrimaryStatsFromLearned(state.PrimaryStats, learned)
}