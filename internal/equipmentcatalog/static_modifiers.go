package equipmentcatalog

import (
	"sort"
	"strings"
)

// StaticModifierID is a stable Server gameplay-stat identifier for fixed item-archetype bonuses.
// It is deliberately separate from random AffixID: fixed archetype power and rolled instance
// quality have different lifecycle and persistence semantics even when they affect the same stat.
type StaticModifierID string

const (
	StaticStrength        StaticModifierID = "strength"
	StaticDexterity       StaticModifierID = "dexterity"
	StaticIntelligence    StaticModifierID = "intelligence"
	StaticConstitution    StaticModifierID = "constitution"
	StaticSpirit          StaticModifierID = "spirit"
	StaticCharisma        StaticModifierID = "charisma"
	StaticPhysicalHit     StaticModifierID = "physical_hit"
	StaticCriticalRating  StaticModifierID = "critical_rating"
	StaticPhysicalDamage  StaticModifierID = "physical_damage"
	StaticMagicPower      StaticModifierID = "magic_power"
	StaticEvasion         StaticModifierID = "evasion"
	StaticMaxHP           StaticModifierID = "max_hp"
	StaticMaxMP           StaticModifierID = "max_mp"
	StaticPhysicalDefense StaticModifierID = "physical_defense"
	StaticMagicDefense    StaticModifierID = "magic_defense"
)

type StaticModifier struct {
	ID    StaticModifierID `json:"stat"`
	Value uint32           `json:"value"`
}

func validStaticModifierID(id StaticModifierID) bool {
	switch id {
	case StaticStrength,
		StaticDexterity,
		StaticIntelligence,
		StaticConstitution,
		StaticSpirit,
		StaticCharisma,
		StaticPhysicalHit,
		StaticCriticalRating,
		StaticPhysicalDamage,
		StaticMagicPower,
		StaticEvasion,
		StaticMaxHP,
		StaticMaxMP,
		StaticPhysicalDefense,
		StaticMagicDefense:
		return true
	default:
		return false
	}
}

func canonicalStaticModifiers(in []StaticModifier) ([]StaticModifier, error) {
	if len(in) == 0 {
		return nil, nil
	}
	canonical := append([]StaticModifier(nil), in...)
	for index := range canonical {
		canonical[index].ID = StaticModifierID(strings.TrimSpace(string(canonical[index].ID)))
		if !validStaticModifierID(canonical[index].ID) || canonical[index].Value == 0 {
			return nil, ErrInvalidCatalog
		}
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].ID == canonical[index].ID {
			return nil, ErrInvalidCatalog
		}
	}
	return canonical, nil
}

func cloneStaticModifiers(in []StaticModifier) []StaticModifier {
	if len(in) == 0 {
		return nil
	}
	return append([]StaticModifier(nil), in...)
}

// StaticModifiersForItem returns a defensive copy of fixed archetype bonuses. Random affixes are
// not included here; callers compose them from the authoritative ItemInstance separately.
func (c *Catalog) StaticModifiersForItem(itemArchetypeID string) ([]StaticModifier, bool) {
	if c == nil {
		return nil, false
	}
	item, ok := c.Resolve(strings.TrimSpace(itemArchetypeID))
	if !ok {
		return nil, false
	}
	return cloneStaticModifiers(item.StaticModifiers), true
}
