package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) prepareAndDispatchAction(name string, command useActionCommand, actorID world.EntityID, tick uint64, delta time.Duration, report *StepReport) {
	prepared, err := r.combat.Prepare(actorID, command.action.ActionID, combat.Target{Kind:combat.TargetKind(command.action.TargetKind),ID:command.action.TargetID}, tick)
	if err != nil {
		if command.action.ActionID == legacyGateActionID && errors.Is(err, combat.ErrActionCooldown) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:command.sessionID,Err:siege.ErrGateAttackCooldown})
			return
		}
		if isExpectedCombatRejection(err) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:command.sessionID,Err:err})
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:command.sessionID,Err:err})
		return
	}
	r.dispatchPreparedAction(name, command, prepared, tick, delta, report)
}

var _ session.ID
