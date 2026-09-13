package worldruntime

import (
	"math"
	"math/rand/v2"
	"strconv"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const basicAttackActionID = "basic-attack"

// weaponAccuracyRoll remains package-private so production owns the 0..9999 basis-point roll.
// Tests in this package may replace it temporarily to make authoritative hit/miss coverage exact.
var weaponAccuracyRoll = func() uint32 { return rand.Uint32N(10000) }

func (r *Runtime) equippedCatalogWeapon(actorID world.EntityID, sourceSessionID session.ID) (equipmentcatalog.Definition, bool) {
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
// by the existing combat cooldown service. Decimal millisecond values can round slightly upward in
// float32, which would make math.Ceil add an unintended whole simulation tick at exact tick
// boundaries. Moving one float32 ULP toward zero preserves the authored millisecond boundary while
// still using the same combat cooldown authority and conservative ceil-to-tick policy.
func weaponAttackCooldownSeconds(milliseconds uint32) float32 {
	if milliseconds == 0 {
		return 0
	}
	seconds := float32(milliseconds) / 1000
	return math.Nextafter32(seconds, 0)
}

// applyEquippedBasicAttackTiming reuses the existing combat cooldown authority. ItemArchetype data
// classifies the weapon only; shared base cadence is resolved from WeaponType. If that type has no
// formally authored interval yet, the prepared action keeps its normal authoritative cooldown.
func (r *Runtime) applyEquippedBasicAttackTiming(prepared *combat.PreparedAction, sourceSessionID session.ID) {
	if prepared == nil || prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return
	}
	definition, ok := r.equippedCatalogWeapon(prepared.ActorEntityID, sourceSessionID)
	if !ok {
		return
	}
	milliseconds, authored := defaultEquipmentCatalog.BasicAttackIntervalMSForItem(definition.ItemArchetypeID)
	if !authored {
		return
	}
	prepared.Definition.CooldownSeconds = weaponAttackCooldownSeconds(milliseconds)
}

func ratingWithSignedBase(base int32, bonus uint32) int32 {
	total := int64(base) + int64(bonus)
	if total > math.MaxInt32 {
		return math.MaxInt32
	}
	if total < math.MinInt32 {
		return math.MinInt32
	}
	return int32(total)
}

func uintRatingAsInt32(value uint32) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func weaponBasicAttackHitChanceBasisPoints(weaponAccuracyModifier int32, physicalHitBonus, targetEvasion uint32) uint32 {
	return combat.PhysicalHitChanceBasisPoints(
		ratingWithSignedBase(weaponAccuracyModifier, physicalHitBonus),
		uintRatingAsInt32(targetEvasion),
	)
}

func weaponBasicAttackHits(weaponAccuracyModifier int32, physicalHitBonus, targetEvasion, rollBasisPoints uint32) bool {
	return rollBasisPoints < weaponBasicAttackHitChanceBasisPoints(weaponAccuracyModifier, physicalHitBonus, targetEvasion)
}

func preparedEntityTargetID(prepared combat.PreparedAction) (world.EntityID, bool) {
	if prepared.Target.Kind != combat.TargetEntity {
		return 0, false
	}
	value, err := strconv.ParseUint(prepared.Target.ID, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return world.EntityID(value), true
}

// resolveEquippedBasicAttackHit applies the formal V1 physical hit/evasion rating formula to an
// entity-target basic attack made with an authored catalog weapon. Attribute-derived ratings are
// not fabricated here; this path consumes the weapon modifier plus currently implemented equipment
// instance affixes, and can accept attribute contributions when the authoritative attribute owner lands.
func (r *Runtime) resolveEquippedBasicAttackHit(actorID world.EntityID, sourceSessionID session.ID, prepared combat.PreparedAction) bool {
	if prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return true
	}
	definition, ok := r.equippedCatalogWeapon(actorID, sourceSessionID)
	if !ok || definition.Weapon == nil {
		return true
	}
	targetID, ok := preparedEntityTargetID(prepared)
	if !ok {
		return false
	}
	attackerModifiers, err := r.equippedInstanceModifiers(actorID)
	if err != nil {
		return false
	}
	targetModifiers, err := r.equippedInstanceModifiers(targetID)
	if err != nil {
		return false
	}
	return weaponBasicAttackHits(
		definition.Weapon.AccuracyModifier,
		attackerModifiers.PhysicalHit,
		targetModifiers.Evasion,
		weaponAccuracyRoll(),
	)
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

// resolveEquippedBasicAttackDamage retains its historical name because it is already the common
// entity-damage hook. Basic attacks first replace the authored placeholder amount with authoritative
// weapon damage. Then all direct entity damage receives the matching equipped unique-instance flat
// modifier: PhysicalDamage for physical damage and MagicPower for magic damage. Attribute-derived
// Strength/Dexterity/Intelligence bonuses are intentionally not fabricated here.
func (r *Runtime) resolveEquippedBasicAttackDamage(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction) uint32 {
	if prepared.Target.Kind != combat.TargetEntity {
		return prepared.Damage.Amount
	}

	damage := prepared.Damage.Amount
	if prepared.Definition.ID == basicAttackActionID {
		if definition, ok := r.equippedCatalogWeapon(actorID, sourceSessionID); ok {
			if weaponDamage := rollWeaponDamage(definition, r.entityWeaponBodySize(targetID), rand.Uint32()); weaponDamage != 0 {
				damage = weaponDamage
			}
		}
	}

	modifiers, err := r.equippedInstanceModifiers(actorID)
	if err != nil {
		return damage
	}
	switch prepared.Damage.Type {
	case combat.DamagePhysical:
		return saturatingAddUint32(damage, modifiers.PhysicalDamage)
	case combat.DamageMagic:
		return saturatingAddUint32(damage, modifiers.MagicPower)
	default:
		return damage
	}
}
