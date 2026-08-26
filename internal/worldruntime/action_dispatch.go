package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

func (r *Runtime) dispatchPreparedAction(name string, sourceSessionID session.ID, prepared combat.PreparedAction, tick uint64, delta time.Duration, report *StepReport) {
	actor, ok := r.world.Entity(prepared.ActorEntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: ErrSessionEntityNotFound})
		return
	}
	cooldownReadyTick := combat.CooldownReadyTick(prepared.Definition, tick, delta)
	switch prepared.Target.Kind {
	case combat.TargetGate:
		if r.siege == nil || r.dynamic == nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: ErrSiegeUnavailable})
			return
		}
		gateState, err := r.siege.ApplyActionDamage(actor.Transform.Position, prepared.Target.ID, prepared.Definition.Range, prepared.Damage, r.dynamic)
		if err != nil {
			if isExpectedGateRejection(err) {
				report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sourceSessionID, Err: err})
			} else {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
			}
			return
		}
		// ApplyActionDamage has completed all gate legality checks. Queue the v10 acceptance cue
		// before the later dynamic-state replication makes the outcome observable to clients.
		r.emitActionStarted(actor.ID, prepared, tick, report)
		r.combat.Commit(prepared, tick, delta)
		if prepared.Definition.Effect == combat.EffectDamage {
			r.cancelReviveProtectionByDamageAction(actor.ID, report)
		}
		r.siege.ObserveGateState(gateState)
		r.bumpDynamicRevision()
	case combat.TargetEntity:
		if r.applyEntityAction(name, sourceSessionID, actor, prepared, prepared, tick, cooldownReadyTick, report) {
			r.combat.Commit(prepared, tick, delta)
			if prepared.Definition.Effect == combat.EffectDamage {
				r.cancelReviveProtectionByDamageAction(actor.ID, report)
			}
		}
	case combat.TargetPoint:
		if r.applyPointAction(name, sourceSessionID, actor, prepared, tick, cooldownReadyTick, report) {
			r.combat.Commit(prepared, tick, delta)
			if prepared.Definition.Effect == combat.EffectDamage {
				r.cancelReviveProtectionByDamageAction(actor.ID, report)
			}
		}
	default:
		report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sourceSessionID, Err: combat.ErrTargetNotAllowed})
	}
}

func isExpectedCombatRejection(err error) bool {
	return errors.Is(err, combat.ErrUnknownAction) || errors.Is(err, combat.ErrTargetNotAllowed) || errors.Is(err, combat.ErrActionCooldown)
}
func isExpectedGateRejection(err error) bool {
	return errors.Is(err, siege.ErrUnknownGate) || errors.Is(err, siege.ErrGateDestroyed) || errors.Is(err, siege.ErrGateWrongLayer) || errors.Is(err, siege.ErrGateOutOfRange) || errors.Is(err, siege.ErrGateNoLineOfSight)
}
