package characterstats

import "github.com/li41/astrahold-server/internal/skillcatalog"

const authoredPassivePrimaryBonus uint32 = 5

// PrimaryBonusForSkill returns the formally authored primary-attribute bonus
// for a classless skill. A false result means that no primary-attribute bonus
// is currently authored for that skill; callers must not invent one.
func PrimaryBonusForSkill(skillID skillcatalog.ID) (AdditiveBonus, bool) {
	switch skillID {
	case skillcatalog.StrongPhysique:
		return AdditiveBonus{Strength: authoredPassivePrimaryBonus}, true
	case skillcatalog.ClearMeridians:
		return AdditiveBonus{Agility: authoredPassivePrimaryBonus}, true
	default:
		return AdditiveBonus{}, false
	}
}
