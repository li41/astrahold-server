package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/actionpolicy"
	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// validateActionResourceCost runs after target/range/LOS legality has passed and before any
// action resource is mutated. Resource legality is independent of retired fixed-profession identity.
func (r *Runtime) validateActionResourceCost(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	prepared combat.PreparedAction,
	targetKind protocol.ActionTargetKind,
	tick uint64,
	report *StepReport,
) bool {
	policy, ok := actionpolicy.ForAction(prepared.Definition.ID)
	if !ok || policy.CostResource == actionresource.Empty || policy.CostAmount == 0 {
		return true
	}
	state, ok := r.characters.State(actorID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: character.ErrCharacterNotFound})
		return false
	}
	resource := state.ActionResource()
	if resource.ID != policy.CostResource || resource.Max == 0 {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: actionresource.ErrResourceMismatch})
		return false
	}
	if resource.Current < policy.CostAmount {
		r.rejectClientAction(name, sourceSessionID, clientActionSequence, actorID, prepared.Definition.ID, targetKind, character.ErrInsufficientResource, tick, report)
		return false
	}
	return true
}

// consumeActionResource mutates the Server-owned classless action resource after all legality gates
// pass. The retired Type117 presentation lane is intentionally not replaced here; gameplay legality
// and mutation remain Server-owned even when no generic action-resource UI snapshot is published.
func (r *Runtime) consumeActionResource(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	prepared combat.PreparedAction,
	targetKind protocol.ActionTargetKind,
	tick uint64,
	report *StepReport,
) bool {
	policy, ok := actionpolicy.ForAction(prepared.Definition.ID)
	if !ok || policy.CostResource == actionresource.Empty || policy.CostAmount == 0 {
		return true
	}
	if _, err := r.characters.SpendActionResource(actorID, policy.CostResource, policy.CostAmount); err != nil {
		if errors.Is(err, character.ErrInsufficientResource) || errors.Is(err, character.ErrCharacterDefeated) {
			r.rejectClientAction(name, sourceSessionID, clientActionSequence, actorID, prepared.Definition.ID, targetKind, err, tick, report)
			return false
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
		return false
	}
	return true
}

// applyAcceptedActionResourceReduction clamps at zero, so a cooling/relief action remains usable
// when the current burden is below the authored reduction amount. This is distinct from a legality cost.
func (r *Runtime) applyAcceptedActionResourceReduction(
	name string,
	sourceSessionID session.ID,
	actorID world.EntityID,
	actionID string,
	report *StepReport,
) bool {
	policy, ok := actionpolicy.ForAction(actionID)
	if !ok || policy.AcceptedReductionResource == actionresource.Empty || policy.AcceptedReductionAmount == 0 {
		return true
	}
	state, ok := r.characters.State(actorID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: character.ErrCharacterNotFound})
		return false
	}
	resource := state.ActionResource()
	if resource.ID != policy.AcceptedReductionResource || resource.Max == 0 {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: actionresource.ErrResourceMismatch})
		return false
	}
	amount := policy.AcceptedReductionAmount
	if amount > resource.Current {
		amount = resource.Current
	}
	if amount > 0 {
		if _, err := r.characters.SpendActionResource(actorID, policy.AcceptedReductionResource, amount); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
			return false
		}
	}
	return true
}

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
