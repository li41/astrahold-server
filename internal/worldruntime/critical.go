package worldruntime

import (
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

// criticalRoll is Server-owned. Tests may replace it inside this package to lock probability
// boundaries without moving critical authority to Client or presentation code.
var criticalRoll = func() uint32 { return rand.Uint32N(10000) }

// resolveCritical decides one direct entity-damage instance independently. Attribute-derived
// critical bonus is intentionally zero until the authoritative primary-stat owner is wired into
// runtime; equipped CriticalRating is already authoritative and participates now.
func (r *Runtime) resolveCritical(actorID world.EntityID, prepared combat.PreparedAction) (bool, error) {
	if !prepared.Definition.CriticalEligible || prepared.Definition.Effect != combat.EffectDamage {
		return false, nil
	}
	modifiers, err := r.equippedInstanceModifiers(actorID)
	if err != nil {
		return false, err
	}
	chance := combat.CriticalChanceBasisPoints(0, modifiers.CriticalRating)
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
