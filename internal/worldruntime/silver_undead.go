package worldruntime

import (
	"math"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	silverUndeadDamageNumeratorV1   uint64 = 6
	silverUndeadDamageDenominatorV1 uint64 = 5
)

// silverUndeadRawPhysicalDamageV1 applies the formal +20% raw-damage material rule using integer
// arithmetic, so the required floor happens before critical, mitigation and block processing.
func silverUndeadRawPhysicalDamageV1(raw uint32) uint32 {
	total := uint64(raw) * silverUndeadDamageNumeratorV1 / silverUndeadDamageDenominatorV1
	if total > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(total)
}

func silverUndeadBasicAttackDamageV1(material equipmentcatalog.MaterialID, classification world.EntityClassification, actionID string, damageType combat.DamageType, raw uint32) uint32 {
	if raw == 0 || material != equipmentcatalog.MaterialSilver || classification != world.EntityClassificationUndead || actionID != basicAttackActionID || damageType != combat.DamagePhysical {
		return raw
	}
	return silverUndeadRawPhysicalDamageV1(raw)
}

// applySilverUndeadBasicAttackBonus resolves both sides from current Server authority: the player's
// actually equipped main-hand archetype and the target's current EntityState classification. No
// Client asset/name/tag participates in the outcome, and off-hand silver never qualifies.
func (r *Runtime) applySilverUndeadBasicAttackBonus(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction, raw uint32) uint32 {
	if r == nil || raw == 0 {
		return raw
	}
	weapon, ok := r.equippedCatalogWeapon(actorID, sourceSessionID)
	if !ok {
		return raw
	}
	target, ok := r.world.Entity(targetID)
	if !ok {
		return raw
	}
	return silverUndeadBasicAttackDamageV1(weapon.Material, target.Classification, prepared.Definition.ID, prepared.Damage.Type, raw)
}
