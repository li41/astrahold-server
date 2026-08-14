package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/deathpenalty"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrDeathRevisionOverflow = errors.New("worldruntime: death revision overflow")

type deathOutcome struct {
	EntityID     world.EntityID
	Revision     uint64
	Context      respawnpolicy.DeathContext
	DefeatedTick uint64
}

func WithDeathPenalty(policy *deathpenalty.Service) Option {
	return func(r *Runtime) { r.deathPenalty = policy }
}

// recordPlayerDefeat 是 player alive -> defeated transition 的單一 outcome boundary。
// ordering刻意固定為：
//
//  1. 產生單調 DefeatRevision；
//  2. 先讓 respawn policy 綁定本次 context/checkpoint/destination/due tick；
//  3. 再 exactly-once 套用 death penalty。
//
// 因此 checkpoint forfeiture 不會改寫「這次死亡」已綁定的 respawn 目的地，只影響之後的死亡。
func (r *Runtime) recordPlayerDefeat(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) {
	outcome, err := r.beginDeathOutcome(entityID, tick, context, report)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "record_death_outcome", Err: err})
		// Revision overflow 不應阻止既有 respawn safety net；即使 penalty seam故障，角色仍要能復活。
		r.scheduleRespawnForDefeat(entityID, tick, context, report)
		return
	}

	r.scheduleRespawnForDefeat(entityID, tick, context, report)
	r.applyDeathPenalty(outcome, report)
}

func (r *Runtime) beginDeathOutcome(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) (deathOutcome, error) {
	current := r.deathRevision[entityID]
	if current == ^uint64(0) {
		return deathOutcome{}, ErrDeathRevisionOverflow
	}
	revision := current + 1
	r.deathRevision[entityID] = revision
	if report != nil {
		report.Metrics.DeathOutcomesRecorded++
	}
	return deathOutcome{EntityID: entityID, Revision: revision, Context: context, DefeatedTick: tick}, nil
}

func (r *Runtime) applyDeathPenalty(outcome deathOutcome, report *StepReport) {
	if r.deathPenalty == nil {
		return
	}
	decision, applied, err := r.deathPenalty.Apply(outcome.EntityID, outcome.Revision, outcome.Context)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "apply_death_penalty", Err: err})
		return
	}
	if !applied {
		return
	}
	if report != nil {
		report.Metrics.DeathPenaltyTransactionsApplied++
	}
	if !decision.ForfeitCheckpoint || r.respawnPolicy == nil {
		return
	}
	if _, ok := r.respawnPolicy.Checkpoint(outcome.EntityID); !ok {
		return
	}
	r.respawnPolicy.ClearCheckpoint(outcome.EntityID)
	if report != nil {
		report.Metrics.DeathPenaltyCheckpointForfeits++
	}
}

func (r *Runtime) clearDeathOutcomeState(entityID world.EntityID) {
	delete(r.deathRevision, entityID)
	if r.deathPenalty != nil {
		r.deathPenalty.Remove(entityID)
	}
}
