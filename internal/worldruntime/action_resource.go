package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// validateActionClassResourceCost runs after target/range/LOS legality has passed and before any
// action resource is mutated. It keeps a future mixed MP + class-resource cost from partially
// spending one resource before discovering that the other is insufficient.
func (r *Runtime) validateActionClassResourceCost(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	prepared combat.PreparedAction,
	targetKind protocol.ActionTargetKind,
	tick uint64,
	report *StepReport,
) bool {
	policy, ok := classaction.ForAction(prepared.Definition.ID)
	if !ok || policy.CostResource == classresource.Empty || policy.CostAmount == 0 {
		return true
	}
	state, ok := r.characters.State(actorID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: character.ErrCharacterNotFound})
		return false
	}
	if state.ClassResourceID != policy.CostResource || state.MaxClassResource == 0 {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: classresource.ErrResourceMismatch})
		return false
	}
	if state.ClassResource < policy.CostAmount {
		r.rejectClientAction(name, sourceSessionID, clientActionSequence, actorID, prepared.Definition.ID, targetKind, character.ErrInsufficientResource, tick, report)
		return false
	}
	return true
}

// consumeActionClassResource commits a prevalidated class-resource cost immediately before an
// action becomes accepted. Complete authoritative class-resource state is then re-sent to the
// owning session; the Client never subtracts a local cost as gameplay truth.
func (r *Runtime) consumeActionClassResource(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	prepared combat.PreparedAction,
	targetKind protocol.ActionTargetKind,
	tick uint64,
	report *StepReport,
) bool {
	policy, ok := classaction.ForAction(prepared.Definition.ID)
	if !ok || policy.CostResource == classresource.Empty || policy.CostAmount == 0 {
		return true
	}
	if _, err := r.characters.SpendClassResource(actorID, policy.CostResource, policy.CostAmount); err != nil {
		if errors.Is(err, character.ErrInsufficientResource) || errors.Is(err, character.ErrCharacterDefeated) {
			r.rejectClientAction(name, sourceSessionID, clientActionSequence, actorID, prepared.Definition.ID, targetKind, err, tick, report)
			return false
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
		return false
	}
	if sourceSession, ok := r.sessions.Get(sourceSessionID); ok && sourceSession.EntityID == actorID {
		r.sendCurrentClassResourceState(sourceSession, report)
	}
	return true
}

// applyAcceptedActionClassResourceReduction commits a Server-authored reduction that is not a
// legality cost. The amount clamps at zero, so a cooling/relief action remains usable when the
// current burden is below the authored reduction amount. This is deliberately distinct from
// CostResource, whose insufficiency rejects the action.
func (r *Runtime) applyAcceptedActionClassResourceReduction(
	name string,
	sourceSessionID session.ID,
	actorID world.EntityID,
	actionID string,
	report *StepReport,
) bool {
	policy, ok := classaction.ForAction(actionID)
	if !ok || policy.AcceptedReductionResource == classresource.Empty || policy.AcceptedReductionAmount == 0 {
		return true
	}
	state, ok := r.characters.State(actorID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: character.ErrCharacterNotFound})
		return false
	}
	if state.ClassResourceID != policy.AcceptedReductionResource || state.MaxClassResource == 0 {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: classresource.ErrResourceMismatch})
		return false
	}
	amount := policy.AcceptedReductionAmount
	if amount > state.ClassResource {
		amount = state.ClassResource
	}
	if amount > 0 {
		if _, err := r.characters.SpendClassResource(actorID, policy.AcceptedReductionResource, amount); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
			return false
		}
	}
	if sourceSession, ok := r.sessions.Get(sourceSessionID); ok && sourceSession.EntityID == actorID {
		r.sendCurrentClassResourceState(sourceSession, report)
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
