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

// silverUndeadBasicAttackDamageWithAmmunitionV1 extends the same material rule to the exact fired
// ammunition. Silver on either the authoritative main hand or the consumed arrow qualifies, but the
// +20% multiplier is applied at most once even if both sources are silver.
func silverUndeadBasicAttackDamageWithAmmunitionV1(weaponMaterial, ammunitionMaterial equipmentcatalog.MaterialID, classification world.EntityClassification, actionID string, damageType combat.DamageType, raw uint32) uint32 {
	if weaponMaterial != equipmentcatalog.MaterialSilver && ammunitionMaterial != equipmentcatalog.MaterialSilver {
		return raw
	}
	return silverUndeadBasicAttackDamageV1(equipmentcatalog.MaterialSilver, classification, actionID, damageType, raw)
}

// applySilverUndeadBasicAttackBonus resolves weapon and target from current Server authority.
// This compatibility path carries no ammunition material and therefore preserves the historical
// silver-main-hand behavior for non-ammunition callers.
func (r *Runtime) applySilverUndeadBasicAttackBonus(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction, raw uint32) uint32 {
	return r.applySilverUndeadBasicAttackBonusWithAmmunition(actorID, sourceSessionID, targetID, prepared, "", raw)
}

// applySilverUndeadBasicAttackBonusWithAmmunition applies the shared silver-vs-undead rule from
// either the actual equipped main-hand material or the exact fired ammunition material. Client
// names, assets, tags and post-consumption inventory contents never participate in the decision.
func (r *Runtime) applySilverUndeadBasicAttackBonusWithAmmunition(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction, ammunitionMaterial equipmentcatalog.MaterialID, raw uint32) uint32 {
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
	return silverUndeadBasicAttackDamageWithAmmunitionV1(weapon.Material, ammunitionMaterial, target.Classification, prepared.Definition.ID, prepared.Damage.Type, raw)
}
