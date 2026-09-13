// Package equipmentstats derives gameplay modifiers from equipped authoritative item instances.
// It owns no mutable world state; callers provide the currently equipped instances.
package equipmentstats

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
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

func Aggregate(instances ...iteminstance.Instance) (Modifiers, error) {
	var result Modifiers
	for _, instance := range instances {
		for _, affix := range instance.Affixes {
			if err := add(&result, affix); err != nil {
				return Modifiers{}, err
			}
		}
	}
	return result, nil
}

func add(result *Modifiers, affix equipmentaffix.Affix) error {
	if result == nil {
		return nil
	}
	value := affix.Value
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
	if *target > math.MaxUint32-value {
		return ErrOverflow
	}
	*target += value
	return nil
}
