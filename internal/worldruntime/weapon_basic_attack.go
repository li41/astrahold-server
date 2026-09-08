package worldruntime

import (
	"math"
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	basicAttackActionID = "basic-attack"

	baseWeaponHitChancePercent int64 = 90
	accuracyModifierStepPercent int64 = 2
	minWeaponHitChancePercent   int64 = 75
	maxWeaponHitChancePercent   int64 = 98
)

// weaponAccuracyRoll remains package-private so production always owns the 0..99 roll. Tests in
// this package may replace it temporarily to make authoritative hit/miss integration coverage exact.
var weaponAccuracyRoll = func() uint32 { return rand.Uint32N(100) }

func (r *Runtime) equippedLowTierWeapon(actorID world.EntityID, sourceSessionID session.ID) (equipmentcatalog.Definition, bool) {
	if r == nil || sourceSessionID == 0 {
		return equipmentcatalog.Definition{}, false
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() {
		return equipmentcatalog.Definition{}, false
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		return equipmentcatalog.Definition{}, false
	}
	definition, ok := defaultEquipmentCatalog.Resolve(inv.MainHand())
	if !ok || definition.Kind != equipmentcatalog.KindWeapon || definition.Weapon == nil {
		return equipmentcatalog.Definition{}, false
	}
	return definition, true
}

// weaponAttackCooldownSeconds converts exact authored milliseconds into the float32 duration used
// by the existing combat cooldown service. Values such as 0.85 and 1.10 can round slightly upward
// in float32, which would make math.Ceil add an unintended whole simulation tick at exact tick
// boundaries. Moving one float32 ULP toward zero preserves the authored millisecond boundary while
// still using the same combat cooldown authority and conservative ceil-to-tick policy.
func weaponAttackCooldownSeconds(milliseconds uint32) float32 {
	if milliseconds == 0 {
		return 0
	}
	seconds := float32(milliseconds) / 1000
	return math.Nextafter32(seconds, 0)
}

// applyEquippedBasicAttackTiming reuses the existing combat cooldown authority. It changes only
// entity-target basic attacks with an authored low-tier weapon; gate/siege timing remains untouched.
func (r *Runtime) applyEquippedBasicAttackTiming(prepared *combat.PreparedAction, sourceSessionID session.ID) {
	if prepared == nil || prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return
	}
	definition, ok := r.equippedLowTierWeapon(prepared.ActorEntityID, sourceSessionID)
	if !ok || definition.Weapon.BasicAttackIntervalMS == 0 {
		return
	}
	prepared.Definition.CooldownSeconds = weaponAttackCooldownSeconds(definition.Weapon.BasicAttackIntervalMS)
}

func weaponBasicAttackHitChancePercent(accuracyModifier int32) uint32 {
	chance := baseWeaponHitChancePercent + int64(accuracyModifier)*accuracyModifierStepPercent
	if chance < minWeaponHitChancePercent {
		chance = minWeaponHitChancePercent
	}
	if chance > maxWeaponHitChancePercent {
		chance = maxWeaponHitChancePercent
	}
	return uint32(chance)
}

func weaponBasicAttackHits(accuracyModifier int32, rollPercent uint32) bool {
	return rollPercent < weaponBasicAttackHitChancePercent(accuracyModifier)
}

// resolveEquippedBasicAttackHit applies the v1 accuracy formula only to an entity-target
// basic attack made with an authored low-tier weapon. Legacy/unclassified MainHand content keeps
// its existing hit behavior until that content receives an explicit accuracy policy.
func (r *Runtime) resolveEquippedBasicAttackHit(actorID world.EntityID, sourceSessionID session.ID, prepared combat.PreparedAction) bool {
	if prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return true
	}
	definition, ok := r.equippedLowTierWeapon(actorID, sourceSessionID)
	if !ok || definition.Weapon == nil {
		return true
	}
	return weaponBasicAttackHits(definition.Weapon.AccuracyModifier, weaponAccuracyRoll())
}

func (r *Runtime) entityWeaponBodySize(entityID world.EntityID) equipmentcatalog.BodySize {
	if entity, ok := r.world.Entity(entityID); ok {
		switch entity.BodySize {
		case world.EntityBodySizeSmall:
			return equipmentcatalog.BodySizeSmall
		case world.EntityBodySizeLarge:
			return equipmentcatalog.BodySizeLarge
		case world.EntityBodySizeGiant:
			return equipmentcatalog.BodySizeGiant
		}
	}
	// Only the current authoritative EntityState may classify this incarnation. An unclassified
	// current entity must never inherit a stale size from lifecycle configuration sharing its ID.
	return equipmentcatalog.BodySizeSmall
}

func rollWeaponDamage(definition equipmentcatalog.Definition, size equipmentcatalog.BodySize, roll uint32) uint32 {
	if definition.Weapon == nil {
		return 0
	}
	rangeDef := definition.DamageRangeFor(size)
	if rangeDef.Min == 0 || rangeDef.Max < rangeDef.Min {
		return 0
	}
	span := uint64(rangeDef.Max) - uint64(rangeDef.Min) + 1
	base := uint64(rangeDef.Min) + uint64(roll)%span
	total := base + uint64(definition.Weapon.ExtraDamage)
	if total > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(total)
}

// resolveEquippedBasicAttackDamage is called only after authoritative target legality and the
// Server-owned weapon accuracy roll both succeed. The Client never supplies the roll or damage amount.
func (r *Runtime) resolveEquippedBasicAttackDamage(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction) uint32 {
	if prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return prepared.Damage.Amount
	}
	definition, ok := r.equippedLowTierWeapon(actorID, sourceSessionID)
	if !ok {
		return prepared.Damage.Amount
	}
	damage := rollWeaponDamage(definition, r.entityWeaponBodySize(targetID), rand.Uint32())
	if damage == 0 {
		return prepared.Damage.Amount
	}
	return damage
}
