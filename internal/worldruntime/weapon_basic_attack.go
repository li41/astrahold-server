package worldruntime

import (
	"math"
	"math/rand/v2"
	"strconv"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characterstats"
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
	itemArchetypeID := inv.MainHand()
	if instance, equipped := inv.MainHandInstance(); equipped {
		itemArchetypeID = instance.ItemArchetypeID
	}
	definition, ok := defaultEquipmentCatalog.Resolve(itemArchetypeID)
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

func signedRatingAsInt32(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

func weaponBasicAttackAttackerRating(weaponAccuracyModifier int32, attributePhysicalHit int64, equipmentPhysicalHit uint32) int32 {
	return signedRatingAsInt32(int64(weaponAccuracyModifier) + attributePhysicalHit + int64(equipmentPhysicalHit))
}

func weaponBasicAttackEvasionRating(attributeEvasion, equipmentEvasion uint32) int32 {
	total := uint64(attributeEvasion) + uint64(equipmentEvasion)
	if total > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(total)
}

func weaponBasicAttackHitChanceBasisPoints(
	weaponAccuracyModifier int32,
	attributePhysicalHit int64,
	equipmentPhysicalHit uint32,
	attributeEvasion uint32,
	equipmentEvasion uint32,
) uint32 {
	return combat.PhysicalHitChanceBasisPoints(
		weaponBasicAttackAttackerRating(weaponAccuracyModifier, attributePhysicalHit, equipmentPhysicalHit),
		weaponBasicAttackEvasionRating(attributeEvasion, equipmentEvasion),
	)
}

func weaponBasicAttackHits(
	weaponAccuracyModifier int32,
	attributePhysicalHit int64,
	equipmentPhysicalHit uint32,
	attributeEvasion uint32,
	equipmentEvasion uint32,
	rollBasisPoints uint32,
) bool {
	return rollBasisPoints < weaponBasicAttackHitChanceBasisPoints(
		weaponAccuracyModifier,
		attributePhysicalHit,
		equipmentPhysicalHit,
		attributeEvasion,
		equipmentEvasion,
	)
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
// entity-target basic attack made with an authored catalog weapon. The authoritative character
// owner supplies effective Agility (including learned passive bonuses); its derived hit/evasion
// ratings are combined with the weapon modifier and unique-equipment affixes before one hit roll.
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
	attackerStats, err := r.characterEffectivePrimaryStats(actorID)
	if err != nil {
		return false
	}
	targetStats, err := r.characterEffectivePrimaryStats(targetID)
	if err != nil {
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
		characterstats.PhysicalHitModifier(attackerStats.Agility),
		attackerModifiers.PhysicalHit,
		characterstats.EvasionModifier(targetStats.Agility),
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

func weaponBasicAttackAttributeDamageBonus(attribute characterstats.ID, stats characterstats.Primary) uint32 {
	switch attribute {
	case characterstats.Strength:
		return characterstats.MeleePhysicalDamageBonus(stats.Strength)
	case characterstats.Agility:
		return characterstats.RangedPhysicalDamageBonus(stats.Agility)
	default:
		return 0
	}
}

func (r *Runtime) matchingSkinBasicAttackDamageBonus(actorID world.EntityID, weaponType equipmentcatalog.WeaponType) uint32 {
	if r == nil || actorID == 0 || weaponType == "" {
		return 0
	}
	affinity, ok := appearance.WeaponAffinity(r.characterSkills.appearanceID(actorID))
	if !ok || affinity != weaponType {
		return 0
	}
	return 1
}

// resolveEquippedBasicAttackDamage retains its historical name because it is already the common
// entity-damage hook. Physical basic attacks first replace the authored placeholder amount with
// authoritative weapon damage, then apply only the WeaponType's formally authored primary-attribute
// scaling, then the selected-skin WeaponType affinity flat +1. All direct entity damage finally
// receives the matching equipped unique-instance flat modifier: PhysicalDamage for physical damage
// and MagicPower for magic damage. Critical and target mitigation happen later in the owner path.
func (r *Runtime) resolveEquippedBasicAttackDamage(actorID world.EntityID, sourceSessionID session.ID, targetID world.EntityID, prepared combat.PreparedAction) uint32 {
	if prepared.Target.Kind != combat.TargetEntity {
		return prepared.Damage.Amount
	}

	damage := prepared.Damage.Amount
	if prepared.Definition.ID == basicAttackActionID {
		if definition, ok := r.equippedCatalogWeapon(actorID, sourceSessionID); ok {
			if weaponDamage := rollWeaponDamage(definition, r.entityWeaponBodySize(targetID), rand.Uint32()); weaponDamage != 0 {
				damage = weaponDamage
				if prepared.Damage.Type == combat.DamagePhysical {
					if attribute, authored := defaultEquipmentCatalog.BasicAttackDamageAttributeForItem(definition.ItemArchetypeID); authored {
						if stats, err := r.characterEffectivePrimaryStats(actorID); err == nil {
							damage = saturatingAddUint32(damage, weaponBasicAttackAttributeDamageBonus(attribute, stats))
						}
					}
					damage = saturatingAddUint32(damage, r.matchingSkinBasicAttackDamageBonus(actorID, definition.Weapon.WeaponType))
				}
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
