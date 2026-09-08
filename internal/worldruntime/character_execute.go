package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classaction"
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
		delete(r.respawnVitalsPhases, targetID)
		r.grantReviveProtection(targetID, tick, report)
		r.markEntityVitalsDirty(targetID)
		report.Metrics.EntityActionsApplied++
		r.emitCombatEvent(protocol.CombatEvent{ActionInstanceID: prepared.ActionInstanceID, ActorEntityID: actor.ID, ActionID: prepared.Definition.ID, Result: protocol.CombatEventResurrect, TargetEntityID: targetID, CooldownReadyTick: cooldownReadyTick}, tick, report)
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

		// Accuracy is an accepted-action outcome, not an action rejection. A miss therefore still
		// emits ActionStarted and consumes the normal cooldown, but never enters damage/mitigation,
		// HP mutation, threat, loot contribution, death, shield block resolution, or class-resource gain.
		if !r.resolveEquippedBasicAttackHit(actor.ID, sessionID, prepared) {
			r.emitActionStarted(actor.ID, startPrepared, tick, report)
			r.emitCombatEvent(protocol.CombatEvent{
				ActionInstanceID:  prepared.ActionInstanceID,
				ActorEntityID:     actor.ID,
				ActionID:          prepared.Definition.ID,
				Result:            protocol.CombatEventMiss,
				TargetEntityID:    targetID,
				CooldownReadyTick: cooldownReadyTick,
			}, tick, report)
			return true
		}

		beforeState, ok := r.combatantState(targetID)
		if !ok {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: ErrSessionEntityNotFound})
			return false
		}

		rawDamage := r.resolveEquippedBasicAttackDamage(actor.ID, sessionID, targetID, prepared)
		damageResult, err := r.resolveIncomingDamage(DamageRequest{
			SourceEntityID: actor.ID,
			TargetEntityID: targetID,
			RawDamage:      rawDamage,
			DamageType:     prepared.Damage.Type,
			Blockable:      prepared.Damage.Blockable,
		})
		if err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}

		r.emitActionStarted(actor.ID, startPrepared, tick, report)
		state, err := r.reduceCombatantHP(targetID, damageResult.FinalDamage)
		if err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			return false
		}
		actualDamage := damageResult.FinalDamage
		if actualDamage > beforeState.HP {
			actualDamage = beforeState.HP
		}
		if target.Kind == world.EntityMonster && actualDamage > 0 {
			r.recordMonsterThreatDamage(targetID, actor.ID, sessionID, actualDamage)
			r.recordMonsterLootDamage(targetID, actor.ID, sessionID, actualDamage)
		}
		if state.Defeated {
			if err := r.world.SetMoveInput(targetID, movement.Input{}); err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			}
			if target.Kind == world.EntityPlayer {
				r.recordPlayerDefeat(targetID, tick, classifyDeathContext(actor, target), report)
			}
		}
		r.markEntityVitalsDirty(targetID)
		if policy, ok := classaction.ForAction(prepared.Definition.ID); ok && policy.HitResource != "" && policy.HitGain > 0 {
			if _, err := r.characters.GainClassResource(actor.ID, policy.HitResource, policy.HitGain); err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
			} else if sourceSession, ok := r.sessions.Get(sessionID); ok && sourceSession.EntityID == actor.ID {
				r.sendCurrentClassResourceState(sourceSession, report)
			}
		}
		report.Metrics.EntityActionsApplied++
		r.emitCombatEvent(protocol.CombatEvent{
			ActionInstanceID:  prepared.ActionInstanceID,
			ActorEntityID:     actor.ID,
			ActionID:          prepared.Definition.ID,
			Result:            protocol.CombatEventHit,
			TargetEntityID:    targetID,
			Damage:            damageResult.FinalDamage,
			Blocked:           damageResult.Blocked,
			CooldownReadyTick: cooldownReadyTick,
		}, tick, report)
		return true

	default:
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: combat.ErrInvalidDefinition})
		return false
	}
}
