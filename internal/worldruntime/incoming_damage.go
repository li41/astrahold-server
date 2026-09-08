package worldruntime

import (
	"errors"
	"math"
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

const physicalDefenseScale = 20.0

var ErrUnsupportedIncomingDamageType = errors.New("worldruntime: unsupported incoming damage type")

// DamageRequest is the already-legal Server-owned damage instance entering mitigation.
// DamageType and Blockable are independent: physical damage is not implicitly blockable.
type DamageRequest struct {
	SourceEntityID world.EntityID
	TargetEntityID world.EntityID
	RawDamage      uint32
	DamageType     combat.DamageType
	Blockable      bool
}

// DamageResult is the single authoritative outcome used by HP mutation, combat events and
// contribution accounting. Blocked means the Server block roll succeeded for this instance.
type DamageResult struct {
	FinalDamage uint32
	Blocked     bool
}

func physicalMitigationRate(physicalDefense uint32) float64 {
	if physicalDefense == 0 {
		return 0
	}
	defense := float64(physicalDefense)
	return defense / (defense + physicalDefenseScale)
}

func (r *Runtime) equippedLowTierShield(targetID world.EntityID) (equipmentcatalog.Definition, bool) {
	if r == nil || targetID == 0 {
		return equipmentcatalog.Definition{}, false
	}
	for _, s := range r.sessions.List() {
		if s.EntityID != targetID || !s.CharacterIdentity.Valid() {
			continue
		}
		inv := r.inventories[s.CharacterIdentity.ID]
		if inv == nil {
			return equipmentcatalog.Definition{}, false
		}
		definition, ok := defaultEquipmentCatalog.Resolve(inv.OffHand())
		if !ok || definition.Kind != equipmentcatalog.KindShield || definition.Shield == nil {
			return equipmentcatalog.Definition{}, false
		}
		return definition, true
	}
	return equipmentcatalog.Definition{}, false
}

func (r *Runtime) resolveIncomingDamage(request DamageRequest) (DamageResult, error) {
	var shield *equipmentcatalog.Shield
	if definition, ok := r.equippedLowTierShield(request.TargetEntityID); ok {
		shield = definition.Shield
	}
	roll := uint32(0)
	if request.DamageType == combat.DamagePhysical && request.Blockable && shield != nil && shield.BlockChancePercent > 0 {
		roll = rand.Uint32()
	}
	return resolveDamageMitigation(request, shield, roll)
}

// resolveDamageMitigation is pure so formula and probability boundaries are deterministic in tests.
// Intermediate math stays float64; positive damage is rounded once at the end and has a minimum of 1.
func resolveDamageMitigation(request DamageRequest, shield *equipmentcatalog.Shield, blockRoll uint32) (DamageResult, error) {
	if request.RawDamage == 0 {
		return DamageResult{}, nil
	}

	damage := float64(request.RawDamage)
	blocked := false

	switch request.DamageType {
	case combat.DamagePhysical:
		if shield != nil && shield.PhysicalDefense > 0 {
			damage *= 1 - physicalMitigationRate(shield.PhysicalDefense)
		}
		if shield != nil && request.Blockable && shield.BlockChancePercent > 0 && blockRoll%100 < uint32(shield.BlockChancePercent) {
			blocked = true
			damage *= 1 - float64(shield.BlockDamageReductionPercent)/100
		}
	case combat.DamageMagic:
		if shield != nil && shield.MagicDamageReductionPercent > 0 {
			damage *= 1 - float64(shield.MagicDamageReductionPercent)/100
		}
	default:
		return DamageResult{}, ErrUnsupportedIncomingDamageType
	}

	rounded := math.Round(damage)
	if rounded < 1 {
		rounded = 1
	}
	if rounded > math.MaxUint32 {
		rounded = math.MaxUint32
	}
	return DamageResult{FinalDamage: uint32(rounded), Blocked: blocked}, nil
}
