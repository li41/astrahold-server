package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) applyEntityAction(name string, sessionID session.ID, actor world.EntityState, prepared combat.PreparedAction, tick uint64, report *StepReport) bool {
	targetID, err := r.validateEntityTarget(actor, prepared)
	if err != nil {
		if errors.Is(err, ErrDynamicWorldUnavailable) {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
		} else {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sessionID, Err: err})
		}
		return false
	}
	target, ok := r.world.Entity(targetID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: ErrSessionEntityNotFound})
		return false
	}
	state, err := r.characters.ReduceHP(targetID, prepared.Damage.Amount)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
		return false
	}
	if state.Defeated {
		// Movement input 是 persistent authoritative state。若只拒絕未來 ClientMoveInput，
		// lethal hit 前最後一個方向仍會在 simulation tick 繼續推動角色，因此 defeat transition
		// 必須在同一個 world-owner command phase立即把既有 input清零。
		if err := r.world.SetMoveInput(targetID, movement.Input{}); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
		}
		// Respawn policy只屬於 player lifecycle。Death context由 authoritative actor/target kind推導，
		// Client不提供 PvE/PvP/Siege分類，也不能指定對應目的地或 delay。
		if target.Kind == world.EntityPlayer {
			r.scheduleRespawnForDefeat(targetID, tick, classifyDeathContext(actor, target), report)
		}
	}
	r.markEntityVitalsDirty(targetID)
	report.Metrics.EntityActionsApplied++
	return true
}
