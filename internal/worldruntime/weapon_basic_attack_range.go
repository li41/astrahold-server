package worldruntime

import (
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/session"
)

// applyEquippedBasicAttackRange keeps basic-attack reach on the authoritative Server path.
// ItemArchetype data classifies the weapon only; an authored WeaponType range overrides the
// action definition before target/range legality is evaluated. Types without an override keep
// the action definition's normal range, so melee and staff do not gain reach implicitly.
func (r *Runtime) applyEquippedBasicAttackRange(prepared *combat.PreparedAction, sourceSessionID session.ID) {
	if prepared == nil || prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return
	}
	definition, ok := r.equippedCatalogWeapon(prepared.ActorEntityID, sourceSessionID)
	if !ok {
		return
	}
	attackRange, authored := defaultEquipmentCatalog.BasicAttackRangeForItem(definition.ItemArchetypeID)
	if !authored {
		return
	}
	prepared.Definition.Range = attackRange
}
