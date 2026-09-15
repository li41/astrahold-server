package worldruntime

import (
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

// criticalRoll is Server-owned. Tests may replace it inside this package to lock probability
// boundaries without moving critical authority to Client or presentation code.
var criticalRoll = func() uint32 { return rand.Uint32N(10000) }

// resolveCritical decides one direct entity-damage instance independently. Only actions whose
// Server-owned definition is CriticalEligible may roll. Effective Agility supplies the formal
// attribute critical bonus, while equipped CriticalRating composes as a separate authoritative
// source before the existing 30% cap is applied.
func (r *Runtime) resolveCritical(actorID world.EntityID, prepared combat.PreparedAction) (bool, error) {
	if !prepared.Definition.CriticalEligible || prepared.Definition.Effect != combat.EffectDamage {
		return false, nil
	}
	stats, err := r.characterEffectivePrimaryStats(actorID)
	if err != nil {
		return false, err
	}
	modifiers, err := r.equippedInstanceModifiers(actorID)
	if err != nil {
		return false, err
	}
	attributeBonusBasisPoints := characterstats.AttributeCriticalBonusPercentPoints(stats.Agility) * 100
	chance := combat.CriticalChanceBasisPoints(attributeBonusBasisPoints, modifiers.CriticalRating)
	return criticalRoll() < chance, nil
}

// applyEquippedFlatDamage is used when an action-specific Server policy replaces the authored base
// damage after normal weapon/base resolution. This preserves the formal order: action base/policy
// damage -> equipped flat damage -> critical -> mitigation.
func (r *Runtime) applyEquippedFlatDamage(actorID world.EntityID, damageType combat.DamageType, damage uint32) (uint32, error) {
	modifiers, err := r.equippedInstanceModifiers(actorID)
	if err != nil {
		return 0, err
	}
	switch damageType {
	case combat.DamagePhysical:
		return saturatingAddUint32(damage, modifiers.PhysicalDamage), nil
	case combat.DamageMagic:
		return saturatingAddUint32(damage, modifiers.MagicPower), nil
	default:
		return damage, nil
	}
}
