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
// SourceSessionID is retained only for diagnostics/backpressure attribution, never for actor truth.
func (r *Runtime) prepareAndDispatchAction(name string, sourceSessionID session.ID, intent combat.Intent, tick uint64, delta time.Duration, report *StepReport) {
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
		report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:sourceSessionID,Err:character.ErrCharacterDefeated})
		return
	}

	prepared, err := r.combat.PrepareIntent(intent, tick)
	if err != nil {
		if intent.ActionID == legacyGateActionID && errors.Is(err, combat.ErrActionCooldown) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:sourceSessionID,Err:siege.ErrGateAttackCooldown})
			return
		}
		if isExpectedCombatRejection(err) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action:name,SessionID:sourceSessionID,Err:err})
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:sourceSessionID,Err:err})
		return
	}
	r.dispatchPreparedAction(name, sourceSessionID, prepared, tick, delta, report)
}
