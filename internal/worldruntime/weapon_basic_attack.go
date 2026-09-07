package worldruntime

import (
	"math"
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const basicAttackActionID = "basic-attack"

func (r *Runtime) equippedLowTierWeapon(actorID world.EntityID, sourceSessionID session.ID) (equipmentcatalog.Definition, bool) {
	if r == nil || sourceSessionID == 0 { return equipmentcatalog.Definition{}, false }
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() { return equipmentcatalog.Definition{}, false }
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil { return equipmentcatalog.Definition{}, false }
	definition, ok := defaultEquipmentCatalog.Resolve(inv.MainHand())
	if !ok || definition.Kind != equipmentcatalog.KindWeapon || definition.Weapon == nil { return equipmentcatalog.Definition{}, false }
	return definition, true
}

// applyEquippedBasicAttackTiming reuses the existing combat cooldown authority. It changes only
// entity-target basic attacks with an authored low-tier weapon; gate/siege timing remains untouched.
func (r *Runtime) applyEquippedBasicAttackTiming(prepared *combat.PreparedAction, sourceSessionID session.ID) {
	if prepared == nil || prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity { return }
	definition, ok := r.equippedLowTierWeapon(prepared.ActorEntityID, sourceSessionID)
	if !ok || definition.Weapon.BasicAttackIntervalMS == 0 { return }
	prepared.Definition.CooldownSeconds = float32(definition.Weapon.BasicAttackIntervalMS) / 1000
}

func (r *Runtime) entityWeaponBodySize(entityID world.EntityID) equipmentcatalog.BodySize {
	for i := range r.monsterLifecycles {
		spawn := r.monsterLifecycles[i].config.Spawn
		if spawn.Entity.ID == entityID {
			if spawn.BodySize != "" { return spawn.BodySize }
			break
		}
	}
	// Unclassified content intentionally uses the small compatibility table. This is not an
	// authored classification and must not be presented to the Client as one.
	return equipmentcatalog.BodySizeSmall
}

func rollWeaponDamage(definition equipmentcatalog.Definition, size equipmentcatalog.BodySize, roll uint32) uint32 {
	if definition.Weapon == nil { return 0 }
	rangeDef := definition.DamageRangeFor(size)
	if rangeDef.Min == 0 || rangeDef.Max < rangeDef.Min { return 0 }
	span := uint64(rangeDef.Max) - uint64(rangeDef.Min) + 1
	base := uint64(rangeDef.Min) + uint64(roll)%span
	total := base + uint64(definition.Weapon.ExtraDamage)
	if total > math.MaxUint32 { return math.MaxUint32 }
	return uint32(total)
}

// resolveEquippedBasicAttackDamage is called only after authoritative target legality succeeds.
// The Client never supplies the roll or damage amount.
func (r *Runtime) resolveEquippedBasicAttackDamage(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction) uint32 {
	if prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity { return prepared.Damage.Amount }
	definition, ok := r.equippedLowTierWeapon(actorID, sourceSessionID)
	if !ok { return prepared.Damage.Amount }
	damage := rollWeaponDamage(definition, r.entityWeaponBodySize(targetID), rand.Uint32())
	if damage == 0 { return prepared.Damage.Amount }
	return damage
}
