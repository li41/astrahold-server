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
	Critical                     bool
	Blockable                    bool
	PhysicalDefenseIgnorePercent uint8
	SelfDamageReductionPercent   uint8
	AdditionalPhysicalDefense    uint32
	MagicDefense                 uint32
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

func magicMitigationRate(magicDefense uint32) float64 {
	if magicDefense == 0 {
		return 0
	}
	defense := float64(magicDefense)
	return defense / (defense + physicalDefenseScale)
}

func (r *Runtime) equippedCatalogShield(targetID world.EntityID) (equipmentcatalog.Definition, bool) {
	if r == nil || targetID == 0 {
		return equipmentcatalog.Definition{}, false
	}
	s, ok := r.sessions.GetByEntity(targetID)
	if !ok || !s.CharacterIdentity.Valid() {
		return equipmentcatalog.Definition{}, false
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

func (r *Runtime) resolveIncomingDamage(request DamageRequest, tick uint64) (DamageResult, error) {
	var shield *equipmentcatalog.Shield
	if definition, ok := r.equippedCatalogShield(request.TargetEntityID); ok {
		shield = definition.Shield
	}
	modifiers, err := r.equippedInstanceModifiers(request.TargetEntityID)
	if err != nil {
		return DamageResult{}, err
	}
	request.AdditionalPhysicalDefense = saturatingAddUint32(request.AdditionalPhysicalDefense, modifiers.PhysicalDefense)
	request.MagicDefense = saturatingAddUint32(request.MagicDefense, modifiers.MagicDefense)
	if request.SelfDamageReductionPercent == 0 && r.combat != nil {
		request.SelfDamageReductionPercent = r.combat.SelfDamageReductionPercent(request.TargetEntityID, tick)
	}
	roll := uint32(0)
	if request.DamageType == combat.DamagePhysical && request.Blockable && shield != nil && shield.BlockChancePercent > 0 {
		roll = rand.Uint32()
	}
	return resolveDamageMitigation(request, shield, roll)
}

func saturatingAddUint32(a, b uint32) uint32 {
	total := uint64(a) + uint64(b)
	if total > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(total)
}

// resolveDamageMitigation is pure so formula and probability boundaries are deterministic in tests.
// Critical is multiplied in float64 before every mitigation layer, so odd integer raw damage keeps
// its half-point until the existing single final math.Round. Physical-defense ignore changes only
// the defense term for this damage instance. Self mitigation is a separate Server-owned multiplier.
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
	if request.Critical {
		damage *= float64(combat.CriticalMultiplierV1Numerator) / float64(combat.CriticalMultiplierV1Denominator)
	}
	blocked := false

	switch request.DamageType {
	case combat.DamagePhysical:
		physicalDefense := request.AdditionalPhysicalDefense
		if shield != nil {
			physicalDefense = saturatingAddUint32(physicalDefense, shield.PhysicalDefense)
		}
		if physicalDefense > 0 {
			damage *= 1 - physicalMitigationRateWithIgnore(physicalDefense, request.PhysicalDefenseIgnorePercent)
		}
		if shield != nil && request.Blockable && shield.BlockChancePercent > 0 && blockRoll%100 < uint32(shield.BlockChancePercent) {
			blocked = true
			damage *= 1 - float64(shield.BlockDamageReductionPercent)/100
		}
	case combat.DamageMagic:
		if request.PhysicalDefenseIgnorePercent != 0 {
			return DamageResult{}, ErrInvalidPhysicalDefenseIgnore
		}
		if request.MagicDefense > 0 {
			damage *= 1 - magicMitigationRate(request.MagicDefense)
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
