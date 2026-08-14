package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/deathpenalty"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrDeathRevisionOverflow = errors.New("worldruntime: death revision overflow")

type deathOutcome struct {
	EntityID                    world.EntityID
	CharacterID                 characteridentity.ID
	CharacterIdentityAssurance  characteridentity.Assurance
	Revision                    uint64
	Context                     respawnpolicy.DeathContext
	DefeatedTick                uint64
}

type deathPenaltyOutcome struct {
	TransactionApplied bool
	CheckpointForfeited bool
}

func WithDeathPenalty(policy *deathpenalty.Service) Option {
	return func(r *Runtime) { r.deathPenalty = policy }
}

func WithDeathOutcomeOutbox(outbox *deathoutcome.Outbox) Option {
	return func(r *Runtime) { r.deathOutbox = outbox }
}

// recordPlayerDefeat 是 player alive -> defeated transition 的單一 outcome boundary。
// ordering刻意固定為：
//
//  1. 產生單調 DefeatRevision 並快照目前 active CharacterID；
//  2. 先讓 respawn policy 綁定本次 context/checkpoint/destination/due tick；
//  3. 再 exactly-once 套用 death penalty；
//  4. 最後把已成立的 immutable outcome enqueue 到 process-local outbox。
//
// 因此 checkpoint forfeiture 不會改寫「這次死亡」已綁定的 respawn 目的地；
// outbox/identity failure也不會 rollback已成立的 lethal / respawn / penalty truth。
func (r *Runtime) recordPlayerDefeat(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) {
	outcome, err := r.beginDeathOutcome(entityID, tick, context, report)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "record_death_outcome", Err: err})
		// Revision overflow 不應阻止既有 respawn safety net；即使 outcome seam故障，角色仍要能復活。
		_, _ = r.scheduleRespawnForDefeat(entityID, tick, context, report)
		return
	}

	scheduled, hasSchedule := r.scheduleRespawnForDefeat(entityID, tick, context, report)
	penalty := r.applyDeathPenalty(outcome, report)
	r.enqueueDeathOutcomeEvent(outcome, scheduled, hasSchedule, penalty, report)
}

func (r *Runtime) beginDeathOutcome(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) (deathOutcome, error) {
	current := r.deathRevision[entityID]
	if current == ^uint64(0) {
		return deathOutcome{}, ErrDeathRevisionOverflow
	}
	revision := current + 1
	r.deathRevision[entityID] = revision
	outcome := deathOutcome{EntityID: entityID, Revision: revision, Context: context, DefeatedTick: tick}
	if binding, ok := r.characterIdentities.binding(entityID); ok {
		outcome.CharacterID = binding.ID
		outcome.CharacterIdentityAssurance = binding.Assurance
	}
	if report != nil {
		report.Metrics.DeathOutcomesRecorded++
	}
	return outcome, nil
}

func (r *Runtime) applyDeathPenalty(outcome deathOutcome, report *StepReport) deathPenaltyOutcome {
	if r.deathPenalty == nil {
		return deathPenaltyOutcome{}
	}
	decision, applied, err := r.deathPenalty.Apply(outcome.EntityID, outcome.Revision, outcome.Context)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "apply_death_penalty", Err: err})
		return deathPenaltyOutcome{}
	}
	if !applied {
		return deathPenaltyOutcome{}
	}
	result := deathPenaltyOutcome{TransactionApplied: true}
	if report != nil {
		report.Metrics.DeathPenaltyTransactionsApplied++
	}
	if !decision.ForfeitCheckpoint || r.respawnPolicy == nil {
		return result
	}
	if _, ok := r.respawnPolicy.Checkpoint(outcome.EntityID); !ok {
		return result
	}
	r.respawnPolicy.ClearCheckpoint(outcome.EntityID)
	result.CheckpointForfeited = true
	if report != nil {
		report.Metrics.DeathPenaltyCheckpointForfeits++
	}
	return result
}

func (r *Runtime) enqueueDeathOutcomeEvent(outcome deathOutcome, scheduled respawnpolicy.Scheduled, hasSchedule bool, penalty deathPenaltyOutcome, report *StepReport) {
	if r.deathOutbox == nil {
		return
	}
	event := deathoutcome.Event{
		EntityID:                   outcome.EntityID,
		CharacterID:                outcome.CharacterID,
		CharacterIdentityAssurance: outcome.CharacterIdentityAssurance,
		DefeatRevision:             outcome.Revision,
		Context:                    outcome.Context,
		DefeatedTick:               outcome.DefeatedTick,
		PenaltyTransactionApplied:  penalty.TransactionApplied,
		CheckpointForfeited:        penalty.CheckpointForfeited,
	}
	if r.respawnPolicy != nil {
		event.RespawnPolicyRevision = r.respawnPolicy.Revision()
	}
	if r.deathPenalty != nil {
		event.DeathPenaltyPolicyRevision = r.deathPenalty.Revision()
	}
	if hasSchedule {
		event.Respawn = deathoutcome.RespawnBinding{
			Scheduled:    true,
			SpawnPointID: scheduled.SpawnPointID,
			SpawnClass:   scheduled.SpawnClass,
			Position:     scheduled.Position,
			DueTick:      scheduled.DueTick,
		}
	}
	_, created, err := r.deathOutbox.Enqueue(event)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "enqueue_death_outcome", Err: err})
		if report != nil {
			report.Metrics.DeathOutcomeEventEnqueueFailures++
		}
		return
	}
	if created && report != nil {
		report.Metrics.DeathOutcomeEventsEnqueued++
	}
}

func (r *Runtime) clearDeathOutcomeState(entityID world.EntityID) {
	delete(r.deathRevision, entityID)
	if r.deathPenalty != nil {
		r.deathPenalty.Remove(entityID)
	}
	if r.deathOutbox != nil {
		r.deathOutbox.ResetEntity(entityID)
	}
}
