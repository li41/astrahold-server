package worldruntime

import (
	"errors"
	"math/rand/v2"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/monstercatalog"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidMonsterCombatStats = errors.New("worldruntime: invalid monster combat stats")

// MonsterCombatStats is immutable authored combat data attached to one current monster incarnation.
// It is stored by EntityID only while that incarnation exists; stable content identity remains the
// monster ArchetypeID carried by world state.
type MonsterCombatStats struct {
	MonsterLevel    uint16
	MaxHP           uint32
	DamageMin       uint32
	DamageMax       uint32
	PhysicalDefense uint32
	MagicDefense    uint32
	PhysicalHit     int32
	Evasion         int32
	CriticalRating  uint32
	BaseXP          uint32
	MeleeActionID   string
}

func monsterCombatStatsFromDefinition(definition monstercatalog.Definition) MonsterCombatStats {
	return MonsterCombatStats{
		MonsterLevel:    definition.MonsterLevel,
		MaxHP:           definition.MaxHP,
		DamageMin:       definition.DamageMin,
		DamageMax:       definition.DamageMax,
		PhysicalDefense: definition.PhysicalDefense,
		MagicDefense:    definition.MagicDefense,
		PhysicalHit:     definition.PhysicalHit,
		Evasion:         definition.Evasion,
		CriticalRating:  definition.CriticalRating,
		BaseXP:          definition.BaseXP,
		MeleeActionID:   definition.MeleeActionID,
	}
}

func validateMonsterCombatStats(stats MonsterCombatStats) error {
	if stats.MonsterLevel == 0 || stats.MaxHP == 0 || stats.DamageMin == 0 || stats.DamageMax < stats.DamageMin || stats.BaseXP == 0 || stats.MeleeActionID == "" || stats.Evasion < 0 {
		return ErrInvalidMonsterCombatStats
	}
	return nil
}

// NewMonsterSpawnEntityRequest converts one validated content definition into the existing
// authoritative static-spawn request. Position and collision capsule still come from the Server map
// owner; Client coordinates or visual scale never enter this conversion.
func NewMonsterSpawnEntityRequest(definition monstercatalog.Definition, entity world.EntityState, radius, maxStepHeight float32) (SpawnEntityRequest, error) {
	if entity.Kind != world.EntityMonster || entity.ArchetypeID != definition.ArchetypeID {
		return SpawnEntityRequest{}, ErrInvalidSpawnEntityRequest
	}
	stats := monsterCombatStatsFromDefinition(definition)
	if err := validateMonsterCombatStats(stats); err != nil {
		return SpawnEntityRequest{}, ErrInvalidSpawnEntityRequest
	}
	request := SpawnEntityRequest{
		Entity:        entity,
		Speed:         definition.MoveSpeed,
		Radius:        radius,
		MaxStepHeight: maxStepHeight,
		HP:            definition.MaxHP,
		MaxHP:         definition.MaxHP,
		BodySize:      definition.BodySize,
		MonsterStats:  stats,
	}
	if err := validateSpawnEntityRequest(request); err != nil {
		return SpawnEntityRequest{}, err
	}
	return request, nil
}

func (r *Runtime) monsterStats(entityID world.EntityID) (MonsterCombatStats, bool) {
	if r == nil || entityID == 0 {
		return MonsterCombatStats{}, false
	}
	stats, ok := r.monsterCombatStats[entityID]
	return stats, ok
}

func (r *Runtime) entityEvasionRating(entityID world.EntityID) (int32, error) {
	if stats, ok := r.monsterStats(entityID); ok {
		return stats.Evasion, nil
	}
	primary, err := r.characterEffectivePrimaryStats(entityID)
	if err != nil {
		return 0, err
	}
	modifiers, err := r.equippedInstanceModifiers(entityID)
	if err != nil {
		return 0, err
	}
	return weaponBasicAttackEvasionRating(characterstats.EvasionModifier(primary.Agility), modifiers.Evasion), nil
}

// monsterDamageRoll is Server-owned. Tests may replace it within this package to lock the authored
// closed interval without moving random authority into Client or content presentation.
var monsterDamageRoll = func() uint32 { return rand.Uint32() }

func rollMonsterDamage(stats MonsterCombatStats, roll uint32) uint32 {
	if stats.DamageMin == 0 || stats.DamageMax < stats.DamageMin {
		return 0
	}
	span := uint64(stats.DamageMax) - uint64(stats.DamageMin) + 1
	return stats.DamageMin + uint32(uint64(roll)%span)
}

func (r *Runtime) resolveMonsterMeleeDamage(actorID world.EntityID, prepared combat.PreparedAction) (uint32, bool) {
	stats, ok := r.monsterStats(actorID)
	if !ok || prepared.Target.Kind != combat.TargetEntity || prepared.Definition.ID != stats.MeleeActionID || prepared.Damage.Type != combat.DamagePhysical {
		return 0, false
	}
	return rollMonsterDamage(stats, monsterDamageRoll()), true
}

// applyAutonomousMeleeActionOverrides keeps cooldown/range legality in the existing Combat Service
// while allowing one shared melee ActionID to serve data-authored monster archetypes.
func (r *Runtime) applyAutonomousMeleeActionOverrides(prepared *combat.PreparedAction) {
	if prepared == nil {
		return
	}
	agent := r.autonomousMeleeAgentForEntity(prepared.ActorEntityID)
	if agent == nil || prepared.Definition.ID != agent.config.ActionID {
		return
	}
	prepared.Definition.Range = agent.config.AttackRange
	if agent.config.AttackIntervalSeconds > 0 {
		prepared.Definition.CooldownSeconds = agent.config.AttackIntervalSeconds
	}
}
