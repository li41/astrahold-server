package worldruntime

import (
	"strconv"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// faceAcceptedClientEntityAction updates gameplay facing only after the authoritative entity action
// path has accepted and applied the action. Rejected intents therefore cannot rotate the actor.
// Server-owned AI keeps its own facing policy and does not enter this client-session helper.
func (r *Runtime) faceAcceptedClientEntityAction(actor world.EntityState, prepared combat.PreparedAction, sourceSessionID session.ID, report *StepReport) {
	if r == nil || sourceSessionID == 0 || prepared.Target.Kind != combat.TargetEntity {
		return
	}
	rawID, err := strconv.ParseUint(prepared.Target.ID, 10, 64)
	if err != nil || rawID == 0 {
		return
	}
	target, ok := r.world.Entity(world.EntityID(rawID))
	if !ok {
		return
	}
	if err := r.world.SetFacingDirection(actor.ID, world.Vec3{
		X: target.Transform.Position.X - actor.Transform.Position.X,
		Z: target.Transform.Position.Z - actor.Transform.Position.Z,
	}); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{
			Command:   "client_action_face",
			SessionID: sourceSessionID,
			Err:       err,
		})
	}
}
