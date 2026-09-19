package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/equipmentstats"
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

func primaryBonusFromEquipmentModifiers(modifiers equipmentstats.Modifiers) characterstats.AdditiveBonus {
	return characterstats.AdditiveBonus{
		Strength: modifiers.Strength,
		Agility: modifiers.Dexterity,
		Constitution: modifiers.Constitution,
		Intelligence: modifiers.Intelligence,
		Spirit: modifiers.Spirit,
		Charisma: modifiers.Charisma,
	}
}

// derivedCharacterMaxVitals is the pure composition seam for the future live level owner.
// It deliberately does not invent a level or mutate current HP/MP; callers must supply the
// authoritative level once progression persistence exists.
func derivedCharacterMaxVitals(level uint32, base characterstats.Primary, learned learnedskills.Set, modifiers equipmentstats.Modifiers) (uint32, uint32, error) {
	effective, err := effectivePrimaryStatsFromLearned(base, learned)
	if err != nil {
		return 0, 0, err
	}
	effective, err = characterstats.Effective(effective, primaryBonusFromEquipmentModifiers(modifiers))
	if err != nil {
		return 0, 0, err
	}
	return characterstats.DerivedMaxVitals(level, effective, modifiers.MaxHP, modifiers.MaxMP)
}

// characterEffectivePrimaryStats is the world-owner read seam for combat scaling. Durable base
// stats are composed with learned passives first, then with currently equipped fixed/rolled primary
// bonuses. Equipment remains derived truth and is never copied into durable base attributes.
func (r *Runtime) characterEffectivePrimaryStats(entityID world.EntityID) (characterstats.Primary, error) {
	state, ok := r.characters.State(entityID)
	if !ok {
		return characterstats.Primary{}, character.ErrCharacterNotFound
	}
	learned, _, err := r.characterSkills.capture(entityID)
	if err != nil {
		return characterstats.Primary{}, err
	}
	effective, err := effectivePrimaryStatsFromLearned(state.PrimaryStats, learned)
	if err != nil {
		return characterstats.Primary{}, err
	}
	modifiers, err := r.equippedInstanceModifiers(entityID)
	if err != nil {
		return characterstats.Primary{}, err
	}
	return characterstats.Effective(effective, primaryBonusFromEquipmentModifiers(modifiers))
}
