package worldruntime

import (
	"strconv"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

// applyPointAction resolves a selected endpoint into the first authoritative combatant
// intersected by the cast line. The source chooses only the endpoint; range, LOS, hit
// selection and damage remain Server-owned.
func (r *Runtime) applyPointAction(name string, sessionID session.ID, actor world.EntityState, prepared combat.PreparedAction, tick uint64, report *StepReport) bool {
	targetID, hit, err := r.resolvePointActionTarget(actor, prepared)
	if err != nil {
		if err == ErrDynamicWorldUnavailable {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sessionID, Err: err})
		} else {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sessionID, Err: err})
		}
		return false
	}
	if !hit {
		// A legal skillshot may miss. It still consumes cooldown in the caller and emits
		// a Server-authored miss so presentation no longer has to infer the outcome.
		x, z := prepared.Target.PointX, prepared.Target.PointZ
		r.emitCombatEvent(protocol.CombatEvent{
			ActionInstanceID: prepared.ActionInstanceID,
			ActorEntityID: actor.ID,
			ActionID: prepared.Definition.ID,
			Result: protocol.CombatEventMiss,
			ImpactX: &x,
			ImpactZ: &z,
		}, tick, report)
		return true
	}

	resolved := prepared
	resolved.Target = combat.Target{
		Kind: combat.TargetEntity,
		ID:   strconv.FormatUint(uint64(targetID), 10),
	}
	return r.applyEntityAction(name, sessionID, actor, resolved, tick, report)
}

func (r *Runtime) resolvePointActionTarget(actor world.EntityState, prepared combat.PreparedAction) (world.EntityID, bool, error) {
	if !prepared.Target.HasPoint || prepared.Definition.Effect != combat.EffectDamage {
		return 0, false, combat.ErrTargetNotAllowed
	}

	start := actor.Transform.Position
	end := world.Position{
		X:     prepared.Target.PointX,
		Y:     start.Y,
		Z:     prepared.Target.PointZ,
		Layer: start.Layer,
	}
	dx := end.X - start.X
	dz := end.Z - start.Z
	lineLengthSq := dx*dx + dz*dz
	if lineLengthSq > prepared.Definition.Range*prepared.Definition.Range {
		return 0, false, ErrPointOutOfRange
	}
	if r.dynamic == nil {
		return 0, false, ErrDynamicWorldUnavailable
	}
	if !r.dynamic.HasLineOfSight(start, end) {
		return 0, false, ErrPointNoLineOfSight
	}
	if lineLengthSq <= 0.000001 {
		return 0, false, nil
	}

	hitRadiusSq := prepared.Definition.HitRadius * prepared.Definition.HitRadius
	candidates := r.world.QueryAOI(
		start,
		prepared.Definition.Range+prepared.Definition.HitRadius,
		spatial.QueryOptions{SameLayer: true},
	)

	bestID := world.EntityID(0)
	bestT := float32(2)
	bestDistanceSq := float32(0)
	for _, candidate := range candidates {
		if candidate.ID == actor.ID || !combatantKind(candidate.Kind) {
			continue
		}
		state, ok := r.combatantState(candidate.ID)
		if !ok || state.Defeated {
			continue
		}

		wx := candidate.Transform.Position.X - start.X
		wz := candidate.Transform.Position.Z - start.Z
		t := (wx*dx + wz*dz) / lineLengthSq
		if t < 0 || t > 1 {
			continue
		}
		closestX := start.X + t*dx
		closestZ := start.Z + t*dz
		cx := candidate.Transform.Position.X - closestX
		cz := candidate.Transform.Position.Z - closestZ
		distanceSq := cx*cx + cz*cz
		if distanceSq > hitRadiusSq {
			continue
		}

		if bestID == 0 || t < bestT || (t == bestT && (distanceSq < bestDistanceSq || (distanceSq == bestDistanceSq && candidate.ID < bestID))) {
			bestID = candidate.ID
			bestT = t
			bestDistanceSq = distanceSq
		}
	}

	return bestID, bestID != 0, nil
}
