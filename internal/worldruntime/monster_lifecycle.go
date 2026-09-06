package worldruntime

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidMonsterLifecycle = errors.New("worldruntime: invalid monster lifecycle")

// MonsterLifecycleConfig owns the authoritative defeat -> corpse -> despawn -> respawn cycle for
// one server-owned monster incarnation. Spawn is the same request used by the normal static spawn
// path so respawn cannot bypass simulation/character registration or invent a second authority.
type MonsterLifecycleConfig struct {
	Spawn             SpawnEntityRequest
	CorpseHoldTicks   uint64
	RespawnDelayTicks uint64
}

type monsterLifecyclePhase uint8

const (
	monsterLifecycleActive monsterLifecyclePhase = iota
	monsterLifecycleCorpse
	monsterLifecycleWaitingRespawn
)

type monsterLifecycle struct {
	config        MonsterLifecycleConfig
	phase         monsterLifecyclePhase
	despawnAtTick uint64
	respawnAtTick uint64
}

func WithMonsterLifecycle(config MonsterLifecycleConfig) Option {
	if err := validateMonsterLifecycleConfig(config); err != nil {
		panic(err)
	}
	return func(r *Runtime) {
		for _, existing := range r.monsterLifecycles {
			if existing.config.Spawn.Entity.ID == config.Spawn.Entity.ID {
				panic(ErrInvalidMonsterLifecycle)
			}
		}
		r.monsterLifecycles = append(r.monsterLifecycles, monsterLifecycle{config: config})
		sort.Slice(r.monsterLifecycles, func(i, j int) bool {
			return r.monsterLifecycles[i].config.Spawn.Entity.ID < r.monsterLifecycles[j].config.Spawn.Entity.ID
		})
	}
}

func validateMonsterLifecycleConfig(config MonsterLifecycleConfig) error {
	if err := validateSpawnEntityRequest(config.Spawn); err != nil {
		return ErrInvalidMonsterLifecycle
	}
	if config.Spawn.Entity.Kind != world.EntityMonster || config.Spawn.HP != config.Spawn.MaxHP {
		return ErrInvalidMonsterLifecycle
	}
	if config.CorpseHoldTicks == 0 || config.RespawnDelayTicks == 0 {
		return ErrInvalidMonsterLifecycle
	}
	return nil
}

// stepMonsterLifecycles runs after the authoritative simulation tick and before replication.
// A lethal combat action therefore gets at least the configured corpse window in world membership,
// allowing EntityVitalsState{Defeated:true} to converge before EntityDespawn is materialized.
func (r *Runtime) stepMonsterLifecycles(tick uint64, report *StepReport) {
	for i := range r.monsterLifecycles {
		lifecycle := &r.monsterLifecycles[i]
		entityID := lifecycle.config.Spawn.Entity.ID
		switch lifecycle.phase {
		case monsterLifecycleActive:
			actor, exists := r.world.Entity(entityID)
			if !exists || actor.Kind != world.EntityMonster {
				continue
			}
			state, exists := r.characters.State(entityID)
			if !exists || !state.Defeated {
				continue
			}
			// Defeat is authoritative immediately. Freeze any stale movement intent even when the
			// defeat source was not the normal damage dispatcher, then keep the corpse in AOI.
			_ = r.world.SetMoveInput(entityID, movement.Input{})
			r.clearAutonomousMeleeTarget(entityID)
			lifecycle.phase = monsterLifecycleCorpse
			lifecycle.despawnAtTick = lifecycleTickAfter(tick, lifecycle.config.CorpseHoldTicks)

		case monsterLifecycleCorpse:
			if tick < lifecycle.despawnAtTick || !r.entityVitalsConverged(entityID) {
				continue
			}
			if _, exists := r.world.Entity(entityID); !exists {
				continue
			}
			r.world.Remove(entityID)
			r.characters.Remove(entityID)
			r.removeEntityVitals(entityID)
			r.clearAutonomousMeleeTarget(entityID)
			lifecycle.phase = monsterLifecycleWaitingRespawn
			lifecycle.respawnAtTick = lifecycleTickAfter(tick, lifecycle.config.RespawnDelayTicks)

		case monsterLifecycleWaitingRespawn:
			if tick < lifecycle.respawnAtTick {
				continue
			}
			// Reusing the same EntityID is safe only after every observer has accepted the old
			// Reliable EntityDespawn. Otherwise the replication view could suppress the new spawn.
			if r.replication.KnownByAny(entityID) {
				continue
			}
			beforeErrors := len(report.CommandErrors)
			r.applySpawnEntity("monster_respawn", lifecycle.config.Spawn, report)
			if len(report.CommandErrors) != beforeErrors {
				continue
			}
			if _, exists := r.world.Entity(entityID); !exists {
				continue
			}
			r.clearAutonomousMeleeTarget(entityID)
			lifecycle.phase = monsterLifecycleActive
			lifecycle.despawnAtTick = 0
			lifecycle.respawnAtTick = 0
		}
	}
}

func (r *Runtime) entityVitalsConverged(entityID world.EntityID) bool {
	revision := r.entityVitalsRevision[entityID]
	if revision == 0 {
		return false
	}
	if _, dirty := r.dirtyVitalsEntities[entityID]; dirty {
		return false
	}
	for _, s := range r.sessions.List() {
		if !r.replication.Knows(s.ID, entityID) {
			continue
		}
		if pending := r.sessionVitalsPending[s.ID]; pending != nil {
			if _, exists := pending[entityID]; exists {
				return false
			}
		}
		delivered := r.sessionVitalsRevision[s.ID]
		if delivered == nil || delivered[entityID] < revision {
			return false
		}
	}
	return true
}

func (r *Runtime) clearAutonomousMeleeTarget(entityID world.EntityID) {
	for i := range r.autonomousMeleeAgents {
		if r.autonomousMeleeAgents[i].config.EntityID == entityID {
			r.autonomousMeleeAgents[i].targetID = 0
			return
		}
	}
}

func lifecycleTickAfter(tick, delay uint64) uint64 {
	if delay > ^uint64(0)-tick {
		return ^uint64(0)
	}
	return tick + delay
}
