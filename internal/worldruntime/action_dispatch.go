package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/siege"
)

func (r *Runtime) dispatchPreparedAction(name string, command useActionCommand, prepared combat.PreparedAction, tick uint64, delta time.Duration, report *StepReport) {
	actorSession, ok := r.sessions.Get(command.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	actor, ok := r.world.Entity(actorSession.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	switch prepared.Target.Kind {
	case combat.TargetGate:
		if r.siege == nil || r.dynamic == nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSiegeUnavailable})
			return
		}
		gateState, err := r.siege.ApplyActionDamage(actor.Transform.Position, prepared.Target.ID, prepared.Definition.Range, prepared.Damage, r.dynamic)
		if err != nil {
			if isExpectedGateRejection(err) {
				report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: err})
			} else {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			}
			return
		}
		r.combat.Commit(prepared, tick, delta)
		if prepared.Definition.Effect == combat.EffectDamage {
			r.cancelReviveProtectionByDamageAction(actor.ID, report)
		}
		r.siege.ObserveGateState(gateState)
		r.bumpDynamicRevision()
	case combat.TargetEntity:
		if r.applyEntityAction(name, command.sessionID, actor, prepared, tick, report) {
			r.combat.Commit(prepared, tick, delta)
			if prepared.Definition.Effect == combat.EffectDamage {
				r.cancelReviveProtectionByDamageAction(actor.ID, report)
			}
		}
	default:
		report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: combat.ErrTargetNotAllowed})
	}
}

func isExpectedCombatRejection(err error) bool {
	return errors.Is(err, combat.ErrUnknownAction) || errors.Is(err, combat.ErrTargetNotAllowed) || errors.Is(err, combat.ErrActionCooldown)
}
func isExpectedGateRejection(err error) bool {
	return errors.Is(err, siege.ErrUnknownGate) || errors.Is(err, siege.ErrGateDestroyed) || errors.Is(err, siege.ErrGateWrongLayer) || errors.Is(err, siege.ErrGateOutOfRange) || errors.Is(err, siege.ErrGateNoLineOfSight)
}
