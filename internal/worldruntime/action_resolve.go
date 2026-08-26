package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/world"
)

func combatIntentFromClientAction(actorID world.EntityID, action protocol.ClientUseAction) combat.Intent {
	target := combat.Target{Kind: combat.TargetKind(action.TargetKind), ID: action.TargetID}
	if action.TargetKind == protocol.ActionTargetPoint && action.TargetX != nil && action.TargetZ != nil {
		target.PointX = *action.TargetX
		target.PointZ = *action.TargetZ
		target.HasPoint = true
	}
	return combat.Intent{ActorEntityID: actorID, ActionID: action.ActionID, Target: target}
}

// prepareAndDispatchAction is transport-neutral after ingress has resolved ActorEntityID.
// SourceSessionID/clientActionSequence are retained only for diagnostics and source-session UX feedback.
func (r *Runtime) prepareAndDispatchAction(name string, sourceSessionID session.ID, clientActionSequence uint32, intent combat.Intent, tick uint64, delta time.Duration, report *StepReport) {
	actor, ok := r.world.Entity(intent.ActorEntityID)
	if !ok || !combatActorKind(actor.Kind) {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sourceSessionID,Err:ErrSessionEntityNotFound})
		return
	}
	actorState, ok := r.combatantState(intent.ActorEntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sourceSessionID,Err:character.ErrCharacterNotFound})
		return
	}
	if actorState.Defeated {
		r.rejectClientAction(name, sourceSessionID, clientActionSequence, intent.ActorEntityID, intent.ActionID, protocol.ActionTargetKind(intent.Target.Kind), character.ErrCharacterDefeated, tick, report)
		return
	}

	prepared, err := r.combat.PrepareIntent(intent, tick)
	if err != nil {
		if intent.ActionID == legacyGateActionID && errors.Is(err, combat.ErrActionCooldown) {
			r.rejectClientAction(name, sourceSessionID, clientActionSequence, intent.ActorEntityID, intent.ActionID, protocol.ActionTargetKind(intent.Target.Kind), siege.ErrGateAttackCooldown, tick, report)
			return
		}
		if isExpectedCombatRejection(err) {
			r.rejectClientAction(name, sourceSessionID, clientActionSequence, intent.ActorEntityID, intent.ActionID, protocol.ActionTargetKind(intent.Target.Kind), err, tick, report)
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sourceSessionID,Err:err})
		return
	}
	r.dispatchPreparedAction(name, sourceSessionID, clientActionSequence, prepared, tick, delta, report)
}
