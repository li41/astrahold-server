package worldruntime

import (
	"strconv"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// applySelfMitigationAction handles the narrow self-only defensive effect before generic entity
// target validation. Normal damage/resurrect actions continue to reject self-targets.
func (r *Runtime) applySelfMitigationAction(
	name string,
	sessionID session.ID,
	clientActionSequence uint32,
	actor world.EntityState,
	prepared combat.PreparedAction,
	startPrepared combat.PreparedAction,
	tick uint64,
	report *StepReport,
) bool {
	if prepared.Definition.Effect != combat.EffectSelfMitigation || prepared.Target.Kind != combat.TargetEntity {
		r.rejectClientAction(name, sessionID, clientActionSequence, actor.ID, startPrepared.Definition.ID, protocol.ActionTargetKind(startPrepared.Target.Kind), combat.ErrTargetNotAllowed, tick, report)
		return false
	}
	rawID, err := strconv.ParseUint(prepared.Target.ID, 10, 64)
	if err != nil || rawID == 0 || world.EntityID(rawID) != actor.ID {
		r.rejectClientAction(name, sessionID, clientActionSequence, actor.ID, startPrepared.Definition.ID, protocol.ActionTargetKind(startPrepared.Target.Kind), combat.ErrTargetNotAllowed, tick, report)
		return false
	}
	if !r.validateActionClassResourceCost(name, sessionID, clientActionSequence, actor.ID, startPrepared, protocol.ActionTargetKind(startPrepared.Target.Kind), tick, report) {
		return false
	}
	if !r.consumeActionMP(name, sessionID, clientActionSequence, actor.ID, startPrepared, protocol.ActionTargetKind(startPrepared.Target.Kind), tick, report) {
		return false
	}
	if !r.consumeActionClassResource(name, sessionID, clientActionSequence, actor.ID, startPrepared, protocol.ActionTargetKind(startPrepared.Target.Kind), tick, report) {
		return false
	}
	if !r.applyAcceptedActionClassResourceReduction(name, sessionID, actor.ID, prepared.Definition.ID, report) {
		return false
	}

	// combat.Service.Commit runs only after this handler returns true. That commit starts both the
	// cooldown and the Server-owned mitigation window, so rejected intents never create either.
	r.emitActionStarted(actor.ID, startPrepared, tick, report)
	report.Metrics.EntityActionsApplied++
	return true
}
