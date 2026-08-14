package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) applyEntityAction(name string, sessionID session.ID, actor world.EntityState, prepared combat.PreparedAction, tick uint64, report *StepReport) bool {
	targetID, err := r.validateEntityTarget(actor, prepared)
	if err != nil {
		if errors.Is(err, ErrDynamicWorldUnavailable) {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sessionID,Err:err})
		} else {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:sessionID,Err:err})
		}
		return false
	}
	if _, err := r.characters.ReduceHP(targetID, prepared.Damage.Amount); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sessionID,Err:err})
		return false
	}
	r.broadcastEntityVitals(targetID, tick, report)
	return true
}
