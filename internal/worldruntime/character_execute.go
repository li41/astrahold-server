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
func (r *Runtime) applyEntityAction(name string, sessionID session.ID, clientActionSequence uint32, actor world.EntityState, prepared combat.PreparedAction, startPrepared combat.PreparedAction, tick uint64, cooldownReadyTick uint64, report *StepReport) bool {
	targetID, err := r.validateEntityTarget(actor, prepared)
	if err != nil {
		if errors.Is(err, ErrDynamicWorldUnavailable) {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
		} else {
			r.rejectClientAction(name, sessionID, clientActionSequence, actor.ID, startPrepared.Definition.ID, protocol.ActionTargetKind(startPrepared.Target.Kind), err, tick, report)
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
		if !r.consumeActionMP(name, sessionID, clientActionSequence, actor.ID, startPrepared, protocol.ActionTargetKind(startPrepared.Target.Kind), tick, report) {
			return false
		}
		r.emitActionStarted(actor.ID, startPrepared, tick, report)
		if _, err := r.characters.RevivePercent(targetID, prepared.Definition.ReviveHPPercent); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}
		if r.respawnPolicy != nil {
			r.respawnPolicy.Cancel(targetID)
		}
		// Resurrection is an alternate authoritative revive path. Any previously armed local-player
		// restart request belongs to the defeated lifecycle and must not survive this transition.
		delete(r.respawnVitalsPhases, targetID)
		r.grantReviveProtection(targetID, tick, report)
		r.markEntityVitalsDirty(targetID)
		report.Metrics.EntityActionsApplied++
		r.emitCombatEvent(protocol.CombatEvent{
			ActionInstanceID:  prepared.ActionInstanceID,
			ActorEntityID:     actor.ID,
			ActionID:          prepared.Definition.ID,
			Result:            protocol.CombatEventResurrect,
			TargetEntityID:    targetID,
			CooldownReadyTick: cooldownReadyTick,
		}, tick, report)
		return true

	case combat.EffectDamage:
		if target.Kind == world.EntityPlayer && r.isReviveProtected(targetID, tick) {
			r.rejectClientAction(name, sessionID, clientActionSequence, actor.ID, startPrepared.Definition.ID, protocol.ActionTargetKind(startPrepared.Target.Kind), ErrEntityReviveProtected, tick, report)
			report.Metrics.ReviveProtectionDamageBlocks++
			return false
		}
		if !r.consumeActionMP(name, sessionID, clientActionSequence, actor.ID, startPrepared, protocol.ActionTargetKind(startPrepared.Target.Kind), tick, report) {
			return false
		}
		r.emitActionStarted(actor.ID, startPrepared, tick, report)
		state, err := r.reduceCombatantHP(targetID, prepared.Damage.Amount)
		if err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}
		if state.Defeated {
			if target.Kind == world.EntityMonster {
				r.recordMonsterLootOwner(targetID, actor.ID, sessionID)
			}
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
			ActionInstanceID:  prepared.ActionInstanceID,
			ActorEntityID:     actor.ID,
			ActionID:          prepared.Definition.ID,
			Result:            protocol.CombatEventHit,
			TargetEntityID:    targetID,
			Damage:            prepared.Damage.Amount,
			CooldownReadyTick: cooldownReadyTick,
		}, tick, report)
		return true

	default:
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: combat.ErrInvalidDefinition})
		return false
	}
}
