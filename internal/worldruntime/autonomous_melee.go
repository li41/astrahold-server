package worldruntime

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidAutonomousMeleeAgent = errors.New("worldruntime: invalid autonomous melee agent")

// AutonomousMeleeAgentConfig is the smallest Server-owned PvE controller needed for the
// first wolf encounter. It does not create a second combat system: movement is written through
// the existing authoritative simulation input and attacks reuse the existing Combat Service.
// AttackRange is steering/animation spacing only; the Combat Action Catalog revalidates legal
// range, target state and line of sight before damage can be committed.
type AutonomousMeleeAgentConfig struct {
	EntityID        world.EntityID
	Home            world.Position
	ActionID        string
	AggroRange      float32
	LeashRange      float32
	AttackRange     float32
	ReturnTolerance float32
}

type autonomousMeleeAgent struct {
	config        AutonomousMeleeAgentConfig
	targetID      world.EntityID
	returningHome bool
}

func WithAutonomousMeleeAgent(config AutonomousMeleeAgentConfig) Option {
	if err := validateAutonomousMeleeAgentConfig(config); err != nil {
		panic(err)
	}
	return func(r *Runtime) {
		for _, existing := range r.autonomousMeleeAgents {
			if existing.config.EntityID == config.EntityID {
				panic(ErrInvalidAutonomousMeleeAgent)
			}
		}
		r.autonomousMeleeAgents = append(r.autonomousMeleeAgents, autonomousMeleeAgent{config: config})
		sort.Slice(r.autonomousMeleeAgents, func(i, j int) bool {
			return r.autonomousMeleeAgents[i].config.EntityID < r.autonomousMeleeAgents[j].config.EntityID
		})
	}
}

func validateAutonomousMeleeAgentConfig(config AutonomousMeleeAgentConfig) error {
	if config.EntityID == 0 || config.ActionID == "" {
		return ErrInvalidAutonomousMeleeAgent
	}
	if !finiteFloat32(config.Home.X) || !finiteFloat32(config.Home.Y) || !finiteFloat32(config.Home.Z) {
		return ErrInvalidAutonomousMeleeAgent
	}
	if !positiveFiniteFloat32(config.AggroRange) || !positiveFiniteFloat32(config.LeashRange) || !positiveFiniteFloat32(config.AttackRange) {
		return ErrInvalidAutonomousMeleeAgent
	}
	if config.LeashRange < config.AggroRange || config.AttackRange > config.AggroRange {
		return ErrInvalidAutonomousMeleeAgent
	}
	if !finiteFloat32(config.ReturnTolerance) || config.ReturnTolerance < 0 {
		return ErrInvalidAutonomousMeleeAgent
	}
	return nil
}

func (r *Runtime) stepAutonomousMeleeAgents(tick uint64, delta time.Duration, report *StepReport) {
	if r.combat == nil || len(r.autonomousMeleeAgents) == 0 {
		return
	}
	for i := range r.autonomousMeleeAgents {
		r.stepAutonomousMeleeAgent(&r.autonomousMeleeAgents[i], tick, delta, report)
	}
}

func (r *Runtime) stepAutonomousMeleeAgent(agent *autonomousMeleeAgent, tick uint64, delta time.Duration, report *StepReport) {
	actor, ok := r.world.Entity(agent.config.EntityID)
	if !ok {
		agent.targetID = 0
		agent.returningHome = false
		return
	}
	state, ok := r.combatantState(actor.ID)
	if !ok || state.Defeated {
		agent.targetID = 0
		agent.returningHome = false
		r.setAutonomousMove(actor.ID, world.Vec3{}, report)
		return
	}
	if actor.Kind != world.EntityMonster || actor.Transform.Position.Layer != agent.config.Home.Layer {
		agent.targetID = 0
		agent.returningHome = false
		r.setAutonomousMove(actor.ID, world.Vec3{}, report)
		return
	}

	if agent.returningHome {
		r.stepAutonomousMeleeReturnHome(agent, actor, report)
		return
	}

	leashSq := agent.config.LeashRange * agent.config.LeashRange
	if actor.Transform.Position.DistanceXZSquared(agent.config.Home) > leashSq {
		r.beginAutonomousMeleeReturnHome(agent, actor, report)
		return
	}

	var target world.EntityState
	var valid bool
	if agent.targetID != 0 {
		target, valid = r.autonomousMeleeTarget(actor, agent.config, agent.targetID, tick)
		if !valid {
			r.beginAutonomousMeleeReturnHome(agent, actor, report)
			return
		}
	} else {
		target, valid = r.acquireAutonomousMeleeTarget(actor, agent.config, tick)
		if valid {
			agent.targetID = target.ID
		} else {
			toleranceSq := agent.config.ReturnTolerance * agent.config.ReturnTolerance
			if actor.Transform.Position.DistanceXZSquared(agent.config.Home) > toleranceSq {
				r.beginAutonomousMeleeReturnHome(agent, actor, report)
				return
			}
			r.setAutonomousMove(actor.ID, world.Vec3{}, report)
			r.restoreAutonomousMeleeVitalsAtHome(actor.ID, report)
			return
		}
	}

	distanceSq := actor.Transform.Position.DistanceXZSquared(target.Transform.Position)
	attackRangeSq := agent.config.AttackRange * agent.config.AttackRange
	if distanceSq <= attackRangeSq {
		r.setAutonomousMove(actor.ID, world.Vec3{}, report)
		if readyTick := r.combat.ActionCooldownReadyTick(actor.ID, agent.config.ActionID); readyTick != 0 && tick < readyTick {
			return
		}
		intent := combat.Intent{
			ActorEntityID: actor.ID,
			ActionID:      agent.config.ActionID,
			Target: combat.Target{
				Kind: combat.TargetEntity,
				ID:   strconv.FormatUint(uint64(target.ID), 10),
			},
		}
		r.prepareAndDispatchAction("autonomous_melee", 0, 0, intent, tick, delta, report)
		return
	}

	r.setAutonomousMove(actor.ID, world.Vec3{
		X: target.Transform.Position.X - actor.Transform.Position.X,
		Z: target.Transform.Position.Z - actor.Transform.Position.Z,
	}, report)
}

func (r *Runtime) beginAutonomousMeleeReturnHome(agent *autonomousMeleeAgent, actor world.EntityState, report *StepReport) {
	agent.targetID = 0
	agent.returningHome = true
	r.stepAutonomousMeleeReturnHome(agent, actor, report)
}

func (r *Runtime) stepAutonomousMeleeReturnHome(agent *autonomousMeleeAgent, actor world.EntityState, report *StepReport) {
	toleranceSq := agent.config.ReturnTolerance * agent.config.ReturnTolerance
	if actor.Transform.Position.DistanceXZSquared(agent.config.Home) <= toleranceSq {
		r.setAutonomousMove(actor.ID, world.Vec3{}, report)
		r.restoreAutonomousMeleeVitalsAtHome(actor.ID, report)
		agent.returningHome = false
		return
	}
	r.setAutonomousMove(actor.ID, world.Vec3{
		X: agent.config.Home.X - actor.Transform.Position.X,
		Z: agent.config.Home.Z - actor.Transform.Position.Z,
	}, report)
}

func (r *Runtime) restoreAutonomousMeleeVitalsAtHome(entityID world.EntityID, report *StepReport) {
	state, ok := r.combatantState(entityID)
	if !ok || state.Defeated || (state.HP == state.MaxHP && state.MP == state.MaxMP) {
		return
	}
	if _, err := r.characters.RestoreAliveFull(entityID); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "autonomous_melee_restore", Err: err})
		return
	}
	r.markEntityVitalsDirty(entityID)
}

func (r *Runtime) autonomousMeleeTarget(actor world.EntityState, config AutonomousMeleeAgentConfig, targetID world.EntityID, tick uint64) (world.EntityState, bool) {
	if targetID == 0 {
		return world.EntityState{}, false
	}
	target, ok := r.world.Entity(targetID)
	if !ok || target.Kind != world.EntityPlayer || target.Transform.Position.Layer != actor.Transform.Position.Layer {
		return world.EntityState{}, false
	}
	state, ok := r.combatantState(targetID)
	if !ok || state.Defeated || r.isReviveProtected(targetID, tick) {
		return world.EntityState{}, false
	}
	leashSq := config.LeashRange * config.LeashRange
	if target.Transform.Position.DistanceXZSquared(config.Home) > leashSq {
		return world.EntityState{}, false
	}
	if r.dynamic == nil || !r.dynamic.HasLineOfSight(actor.Transform.Position, target.Transform.Position) {
		return world.EntityState{}, false
	}
	return target, true
}

func (r *Runtime) acquireAutonomousMeleeTarget(actor world.EntityState, config AutonomousMeleeAgentConfig, tick uint64) (world.EntityState, bool) {
	aggroSq := config.AggroRange * config.AggroRange
	leashSq := config.LeashRange * config.LeashRange
	var best world.EntityState
	var bestDistance float32
	found := false
	for _, s := range r.sessions.List() {
		target, ok := r.world.Entity(s.EntityID)
		if !ok || target.Kind != world.EntityPlayer || target.Transform.Position.Layer != actor.Transform.Position.Layer {
			continue
		}
		state, ok := r.combatantState(target.ID)
		if !ok || state.Defeated || r.isReviveProtected(target.ID, tick) {
			continue
		}
		if target.Transform.Position.DistanceXZSquared(config.Home) > leashSq {
			continue
		}
		distance := actor.Transform.Position.DistanceXZSquared(target.Transform.Position)
		if distance > aggroSq {
			continue
		}
		if r.dynamic == nil || !r.dynamic.HasLineOfSight(actor.Transform.Position, target.Transform.Position) {
			continue
		}
		if !found || distance < bestDistance || (distance == bestDistance && target.ID < best.ID) {
			best = target
			bestDistance = distance
			found = true
		}
	}
	return best, found
}

func (r *Runtime) setAutonomousMove(entityID world.EntityID, direction world.Vec3, report *StepReport) {
	if err := r.world.SetMoveInput(entityID, movement.Input{Direction: direction}); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "autonomous_melee_move", Err: err})
	}
}

func positiveFiniteFloat32(value float32) bool {
	return value > 0 && finiteFloat32(value)
}

func finiteFloat32(value float32) bool {
	v := float64(value)
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
