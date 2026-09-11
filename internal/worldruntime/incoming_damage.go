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

var (
	ErrUnsupportedIncomingDamageType = errors.New("worldruntime: unsupported incoming damage type")
	ErrInvalidPhysicalDefenseIgnore  = errors.New("worldruntime: invalid physical defense ignore")
	ErrInvalidSelfDamageReduction    = errors.New("worldruntime: invalid self damage reduction")
)

// DamageRequest is the already-legal Server-owned damage instance entering mitigation.
// DamageType and Blockable are independent: physical damage is not implicitly blockable.
type DamageRequest struct {
	SourceEntityID               world.EntityID
	TargetEntityID               world.EntityID
	RawDamage                    uint32
	DamageType                   combat.DamageType
	Blockable                    bool
	PhysicalDefenseIgnorePercent uint8
	SelfDamageReductionPercent   uint8
}

// DamageResult is the single authoritative outcome used by HP mutation, combat events and
// contribution accounting. Blocked means the Server block roll succeeded for this instance.
type DamageResult struct {
	FinalDamage uint32
	Blocked     bool
}

func physicalMitigationRate(physicalDefense uint32) float64 {
	return physicalMitigationRateWithIgnore(physicalDefense, 0)
}

func physicalMitigationRateWithIgnore(physicalDefense uint32, ignorePercent uint8) float64 {
	if physicalDefense == 0 || ignorePercent >= 100 {
		return 0
	}
	effectiveDefense := float64(physicalDefense) * (1 - float64(ignorePercent)/100)
	return effectiveDefense / (effectiveDefense + physicalDefenseScale)
}

func (r *Runtime) equippedLowTierShield(targetID world.EntityID) (equipmentcatalog.Definition, bool) {
	if r == nil || targetID == 0 {
		return equipmentcatalog.Definition{}, false
	}
	// Character identity is already authoritative entity ownership truth inside the world owner.
	// Looking up the binding directly avoids rebuilding/sorting the entire Session registry for
	// every incoming hit while preserving the same player-inventory semantics.
	binding, ok := r.characterIdentities.binding(targetID)
	if !ok || !binding.Valid() {
		return equipmentcatalog.Definition{}, false
	}
	inv := r.inventories[binding.ID]
	if inv == nil {
		return equipmentcatalog.Definition{}, false
	}
	definition, ok := defaultEquipmentCatalog.Resolve(inv.OffHand())
	if !ok || definition.Kind != equipmentcatalog.KindShield || definition.Shield == nil {
		return equipmentcatalog.Definition{}, false
	}
	return definition, true
}

func (r *Runtime) resolveIncomingDamage(request DamageRequest, tick uint64) (DamageResult, error) {
	var shield *equipmentcatalog.Shield
	if definition, ok := r.equippedLowTierShield(request.TargetEntityID); ok {
		shield = definition.Shield
	}
	if request.SelfDamageReductionPercent == 0 && r.combat != nil {
		request.SelfDamageReductionPercent = r.combat.SelfDamageReductionPercent(request.TargetEntityID, tick)
	}
	roll := uint32(0)
	if request.DamageType == combat.DamagePhysical && request.Blockable && shield != nil && shield.BlockChancePercent > 0 {
		roll = rand.Uint32()
	}
	return resolveDamageMitigation(request, shield, roll)
}

// resolveDamageMitigation is pure so formula and probability boundaries are deterministic in tests.
// Intermediate math stays float64; positive damage is rounded once at the end and has a minimum of 1.
// Physical-defense ignore changes only the defense term for this damage instance. Self mitigation is
// a separate Server-owned multiplier and never mutates block, defense, equipment or later damage.
func resolveDamageMitigation(request DamageRequest, shield *equipmentcatalog.Shield, blockRoll uint32) (DamageResult, error) {
	if request.RawDamage == 0 {
		return DamageResult{}, nil
	}
	if request.PhysicalDefenseIgnorePercent > 100 {
		return DamageResult{}, ErrInvalidPhysicalDefenseIgnore
	}
	if request.SelfDamageReductionPercent >= 100 {
		return DamageResult{}, ErrInvalidSelfDamageReduction
	}

	damage := float64(request.RawDamage)
	blocked := false

	switch request.DamageType {
	case combat.DamagePhysical:
		if shield != nil && shield.PhysicalDefense > 0 {
			damage *= 1 - physicalMitigationRateWithIgnore(shield.PhysicalDefense, request.PhysicalDefenseIgnorePercent)
		}
		if shield != nil && request.Blockable && shield.BlockChancePercent > 0 && blockRoll%100 < uint32(shield.BlockChancePercent) {
			blocked = true
			damage *= 1 - float64(shield.BlockDamageReductionPercent)/100
		}
	case combat.DamageMagic:
		if request.PhysicalDefenseIgnorePercent != 0 {
			return DamageResult{}, ErrInvalidPhysicalDefenseIgnore
		}
		if shield != nil && shield.MagicDamageReductionPercent > 0 {
			damage *= 1 - float64(shield.MagicDamageReductionPercent)/100
		}
	default:
		return DamageResult{}, ErrUnsupportedIncomingDamageType
	}

	if request.SelfDamageReductionPercent > 0 {
		damage *= 1 - float64(request.SelfDamageReductionPercent)/100
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
