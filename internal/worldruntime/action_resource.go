package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// consumeActionMP is called only after target/range/LOS legality has passed and immediately
// before an action becomes accepted. A rejection is source-session-only feedback and does not
// mutate MP, cooldown, target HP, or presentation state.
func (r *Runtime) consumeActionMP(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	prepared combat.PreparedAction,
	targetKind protocol.ActionTargetKind,
	tick uint64,
	report *StepReport,
) bool {
	cost := prepared.Definition.MPCost
	if cost == 0 {
		return true
	}
	if _, err := r.characters.SpendMP(actorID, cost); err != nil {
		if err == character.ErrInsufficientResource || err == character.ErrCharacterDefeated {
			r.rejectClientAction(name, sourceSessionID, clientActionSequence, actorID, prepared.Definition.ID, targetKind, err, tick, report)
			return false
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
		return false
	}
	r.markEntityVitalsDirty(actorID)
	return true
}
