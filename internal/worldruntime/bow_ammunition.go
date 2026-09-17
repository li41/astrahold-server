package worldruntime

import (
	"errors"
	"math"
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrBowArrowRequired = errors.Join(errors.New("worldruntime: bow arrow required"), character.ErrInsufficientResource)

func (r *Runtime) selectedBowArrow(actorID world.EntityID, sourceSessionID session.ID) (ammunition.Definition, bool, bool) {
	weapon, ok := r.equippedCatalogWeapon(actorID, sourceSessionID)
	if !ok || weapon.Weapon == nil || weapon.Weapon.WeaponType != equipmentcatalog.WeaponTypeBow {
		return ammunition.Definition{}, false, false
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() {
		return ammunition.Definition{}, true, false
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		return ammunition.Definition{}, true, false
	}
	for _, definition := range ammunition.Definitions() {
		if inv.Quantity(definition.ItemArchetypeID) > 0 {
			return definition, true, true
		}
	}
	return ammunition.Definition{}, true, false
}

// validateBasicAttackAmmunition rejects a bow attack before dispatch when no arrow exists. Because
// combat cooldown is committed only after an accepted dispatch, this rejection consumes neither
// ammo nor cooldown and emits no ActionStarted/CombatEvent. Client presentation may still animate
// its local draw/release attempt; it cannot create an authoritative projectile or damage result.
func (r *Runtime) validateBasicAttackAmmunition(prepared combat.PreparedAction, sourceSessionID session.ID) error {
	if prepared.Definition.ID != basicAttackActionID || prepared.Target.Kind != combat.TargetEntity {
		return nil
	}
	_, required, available := r.selectedBowArrow(prepared.ActorEntityID, sourceSessionID)
	if required && !available {
		return ErrBowArrowRequired
	}
	return nil
}

func (r *Runtime) consumeExactArrow(sourceSessionID session.ID, actorID world.EntityID, definition ammunition.Definition) bool {
	if definition.ItemArchetypeID == "" {
		return false
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() {
		return false
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil || inv.Quantity(definition.ItemArchetypeID) == 0 {
		return false
	}
	if err := inv.Remove(definition.ItemArchetypeID, 1); err != nil {
		return false
	}
	r.sessionInventoryPending[sourceSessionID] = struct{}{}
	return true
}

// finalizeBasicAttackHit consumes one arrow on an actual bow miss. Hit shots leave the selected
// arrow in place until damage resolution so the exact fired arrow can contribute to the one combined
// bow+arrow damage roll before that same arrow is consumed.
func (r *Runtime) finalizeBasicAttackHit(actorID world.EntityID, sourceSessionID session.ID, hit bool) bool {
	if hit {
		return true
	}
	definition, required, available := r.selectedBowArrow(actorID, sourceSessionID)
	if required && available {
		_ = r.consumeExactArrow(sourceSessionID, actorID, definition)
	}
	return false
}

func rollBowAndArrowDamage(definition equipmentcatalog.Definition, size equipmentcatalog.BodySize, arrow ammunition.Definition, roll uint32) uint32 {
	if definition.Weapon == nil || definition.Weapon.WeaponType != equipmentcatalog.WeaponTypeBow {
		return 0
	}
	bow := definition.DamageRangeFor(size)
	if bow.Min == 0 || bow.Max < bow.Min || arrow.Damage.Min == 0 || arrow.Damage.Max < arrow.Damage.Min {
		return 0
	}
	minDamage := uint64(bow.Min) + uint64(arrow.Damage.Min)
	maxDamage := uint64(bow.Max) + uint64(arrow.Damage.Max)
	if minDamage > math.MaxUint32 {
		return math.MaxUint32
	}
	if maxDamage > math.MaxUint32 {
		maxDamage = math.MaxUint32
	}
	span := maxDamage - minDamage + 1
	total := minDamage + uint64(roll)%span + uint64(definition.Weapon.ExtraDamage)
	if total > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(total)
}

// rollEquippedBasicAttackWeaponDamageWithAmmunition keeps all non-bow weapons on their existing
// damage path. Bow hits combine bow + exact selected arrow into one uniform roll, consume that exact
// arrow, and return its material alongside damage so later material rules use the fired ammunition
// rather than guessing from post-consumption inventory state.
func (r *Runtime) rollEquippedBasicAttackWeaponDamageWithAmmunition(actorID world.EntityID, sourceSessionID session.ID, definition equipmentcatalog.Definition, size equipmentcatalog.BodySize) (uint32, equipmentcatalog.MaterialID) {
	if definition.Weapon == nil || definition.Weapon.WeaponType != equipmentcatalog.WeaponTypeBow {
		return rollWeaponDamage(definition, size, rand.Uint32()), ""
	}
	arrow, required, available := r.selectedBowArrow(actorID, sourceSessionID)
	if !required || !available {
		return 0, ""
	}
	damage := rollBowAndArrowDamage(definition, size, arrow, rand.Uint32())
	if damage == 0 || !r.consumeExactArrow(sourceSessionID, actorID, arrow) {
		return 0, ""
	}
	return damage, arrow.Material
}

// rollEquippedBasicAttackWeaponDamage remains the compatibility wrapper for callers that only need
// the materialized weapon damage and do not participate in ammunition-specific material effects.
func (r *Runtime) rollEquippedBasicAttackWeaponDamage(actorID world.EntityID, sourceSessionID session.ID, definition equipmentcatalog.Definition, size equipmentcatalog.BodySize) uint32 {
	damage, _ := r.rollEquippedBasicAttackWeaponDamageWithAmmunition(actorID, sourceSessionID, definition, size)
	return damage
}
