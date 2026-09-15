// Package equipmentstats derives gameplay modifiers from equipped authoritative equipment.
// It owns no mutable world state; callers provide fixed archetype modifiers and item instances.
package equipmentstats

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

var ErrOverflow = errors.New("equipmentstats: modifier overflow")

type Modifiers struct {
	Strength        uint32
	Dexterity       uint32
	Intelligence    uint32
	Constitution    uint32
	Spirit          uint32
	Charisma        uint32
	PhysicalHit     uint32
	CriticalRating  uint32
	PhysicalDamage  uint32
	MagicPower      uint32
	Evasion         uint32
	MaxHP           uint32
	MaxMP           uint32
	PhysicalDefense uint32
	MagicDefense    uint32
}

// Aggregate derives random ItemInstance affix bonuses. Fixed archetype modifiers are deliberately
// handled by AggregateStatic so rerollable instance quality and immutable base item power stay
// separate until the final authoritative equipment composition step.
func Aggregate(instances ...iteminstance.Instance) (Modifiers, error) {
	var result Modifiers
	for _, instance := range instances {
		for _, affix := range instance.Affixes {
			if err := addAffix(&result, affix); err != nil {
				return Modifiers{}, err
			}
		}
	}
	return result, nil
}

func AggregateStatic(modifiers ...equipmentcatalog.StaticModifier) (Modifiers, error) {
	var result Modifiers
	for _, modifier := range modifiers {
		if err := addStatic(&result, modifier); err != nil {
			return Modifiers{}, err
		}
	}
	return result, nil
}

// Merge composes independent modifier sources without wraparound. It is used by the world owner to
// combine immutable archetype bonuses with persisted random affixes before any combat formula reads
// the result.
func Merge(inputs ...Modifiers) (Modifiers, error) {
	var result Modifiers
	for _, input := range inputs {
		pairs := []struct {
			target *uint32
			value  uint32
		}{
			{&result.Strength, input.Strength},
			{&result.Dexterity, input.Dexterity},
			{&result.Intelligence, input.Intelligence},
			{&result.Constitution, input.Constitution},
			{&result.Spirit, input.Spirit},
			{&result.Charisma, input.Charisma},
			{&result.PhysicalHit, input.PhysicalHit},
			{&result.CriticalRating, input.CriticalRating},
			{&result.PhysicalDamage, input.PhysicalDamage},
			{&result.MagicPower, input.MagicPower},
			{&result.Evasion, input.Evasion},
			{&result.MaxHP, input.MaxHP},
			{&result.MaxMP, input.MaxMP},
			{&result.PhysicalDefense, input.PhysicalDefense},
			{&result.MagicDefense, input.MagicDefense},
		}
		for _, pair := range pairs {
			if err := addValue(pair.target, pair.value); err != nil {
				return Modifiers{}, err
			}
		}
	}
	return result, nil
}

func addAffix(result *Modifiers, affix equipmentaffix.Affix) error {
	if result == nil {
		return nil
	}
	var target *uint32
	switch affix.ID {
	case equipmentaffix.AffixStrength:
		target = &result.Strength
	case equipmentaffix.AffixDexterity:
		target = &result.Dexterity
	case equipmentaffix.AffixIntelligence:
		target = &result.Intelligence
	case equipmentaffix.AffixConstitution:
		target = &result.Constitution
	case equipmentaffix.AffixSpirit:
		target = &result.Spirit
	case equipmentaffix.AffixCharisma:
		target = &result.Charisma
	case equipmentaffix.AffixPhysicalHit:
		target = &result.PhysicalHit
	case equipmentaffix.AffixCriticalRating:
		target = &result.CriticalRating
	case equipmentaffix.AffixPhysicalDamage:
		target = &result.PhysicalDamage
	case equipmentaffix.AffixMagicPower:
		target = &result.MagicPower
	case equipmentaffix.AffixEvasion:
		target = &result.Evasion
	case equipmentaffix.AffixMaxHP:
		target = &result.MaxHP
	case equipmentaffix.AffixMaxMP:
		target = &result.MaxMP
	case equipmentaffix.AffixPhysicalDefense:
		target = &result.PhysicalDefense
	case equipmentaffix.AffixMagicDefense:
		target = &result.MagicDefense
	default:
		return nil
	}
	return addValue(target, affix.Value)
}

func addStatic(result *Modifiers, modifier equipmentcatalog.StaticModifier) error {
	if result == nil {
		return nil
	}
	var target *uint32
	switch modifier.ID {
	case equipmentcatalog.StaticStrength:
		target = &result.Strength
	case equipmentcatalog.StaticDexterity:
		target = &result.Dexterity
	case equipmentcatalog.StaticIntelligence:
		target = &result.Intelligence
	case equipmentcatalog.StaticConstitution:
		target = &result.Constitution
	case equipmentcatalog.StaticSpirit:
		target = &result.Spirit
	case equipmentcatalog.StaticCharisma:
		target = &result.Charisma
	case equipmentcatalog.StaticPhysicalHit:
		target = &result.PhysicalHit
	case equipmentcatalog.StaticCriticalRating:
		target = &result.CriticalRating
	case equipmentcatalog.StaticPhysicalDamage:
		target = &result.PhysicalDamage
	case equipmentcatalog.StaticMagicPower:
		target = &result.MagicPower
	case equipmentcatalog.StaticEvasion:
		target = &result.Evasion
	case equipmentcatalog.StaticMaxHP:
		target = &result.MaxHP
	case equipmentcatalog.StaticMaxMP:
		target = &result.MaxMP
	case equipmentcatalog.StaticPhysicalDefense:
		target = &result.PhysicalDefense
	case equipmentcatalog.StaticMagicDefense:
		target = &result.MagicDefense
	default:
		return nil
	}
	return addValue(target, modifier.Value)
}

func addValue(target *uint32, value uint32) error {
	if target == nil || value == 0 {
		return nil
	}
	if *target > math.MaxUint32-value {
		return ErrOverflow
	}
	*target += value
	return nil
}
