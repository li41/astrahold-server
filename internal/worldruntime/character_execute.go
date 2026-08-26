package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// startPrepared preserves the accepted target spec used for ActionStarted. Point actions may resolve
// to an entity for HP mutation while their presentation target must remain the original point.
func (r *Runtime) applyEntityAction(name string, sessionID session.ID, actor world.EntityState, prepared combat.PreparedAction, startPrepared combat.PreparedAction, tick uint64, cooldownReadyTick uint64, report *StepReport) bool {
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

	switch prepared.Definition.Effect {
	case combat.EffectResurrect:
		r.emitActionStarted(actor.ID, startPrepared, tick, report)
		if _, err := r.characters.RevivePercent(targetID, prepared.Definition.ReviveHPPercent); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}
		if r.respawnPolicy != nil {
			r.respawnPolicy.Cancel(targetID)
		}
		r.grantReviveProtection(targetID, tick, report)
		r.markEntityVitalsDirty(targetID)
		report.Metrics.EntityActionsApplied++
		r.emitCombatEvent(protocol.CombatEvent{
			ActionInstanceID: prepared.ActionInstanceID,
			ActorEntityID: actor.ID,
			ActionID: prepared.Definition.ID,
			Result: protocol.CombatEventResurrect,
			TargetEntityID: targetID,
			CooldownReadyTick: cooldownReadyTick,
		}, tick, report)
		return true

	case combat.EffectDamage:
		// Player-only policies stay outside generic combatant HP mutation.
		if target.Kind == world.EntityPlayer && r.isReviveProtected(targetID, tick) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sessionID, Err: ErrEntityReviveProtected})
			report.Metrics.ReviveProtectionDamageBlocks++
			return false
		}
		r.emitActionStarted(actor.ID, startPrepared, tick, report)
		state, err := r.reduceCombatantHP(targetID, prepared.Damage.Amount)
		if err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}
		if state.Defeated {
			// Every movable combatant stops immediately on defeat; only Players receive the
			// durable death/respawn/death-penalty policy below.
			if err := r.world.SetMoveInput(targetID, movement.Input{}); err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			}
			if target.Kind == world.EntityPlayer {
				r.recordPlayerDefeat(targetID, tick, classifyDeathContext(actor, target), report)
			}
		}
		r.markEntityVitalsDirty(targetID)
		report.Metrics.EntityActionsApplied++
		r.emitCombatEvent(protocol.CombatEvent{
			ActionInstanceID: prepared.ActionInstanceID,
			ActorEntityID: actor.ID,
			ActionID: prepared.Definition.ID,
			Result: protocol.CombatEventHit,
			TargetEntityID: targetID,
			Damage: prepared.Damage.Amount,
			CooldownReadyTick: cooldownReadyTick,
		}, tick, report)
		return true

	default:
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: combat.ErrInvalidDefinition})
		return false
	}
}
