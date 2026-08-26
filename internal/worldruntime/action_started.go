package worldruntime

import (
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// emitActionStarted publishes presentation-only action acceptance. The actor always receives its
// acknowledgement; observers receive it only after the actor's Reliable EntitySpawn has entered
// their outbound queue, so clients never need to animate an entity they cannot yet render.
func (r *Runtime) emitActionStarted(actorID world.EntityID, prepared combat.PreparedAction, tick uint64, report *StepReport) {
	if actorID == 0 || prepared.ActionInstanceID == 0 || prepared.Definition.ID == "" {
		return
	}
	started := protocol.ActionStarted{
		ActionInstanceID: prepared.ActionInstanceID,
		ActorEntityID: actorID,
		ActionID: prepared.Definition.ID,
		TargetKind: protocol.ActionTargetKind(prepared.Target.Kind),
		TargetID: prepared.Target.ID,
	}
	if prepared.Target.Kind == combat.TargetPoint && prepared.Target.HasPoint {
		x, z := prepared.Target.PointX, prepared.Target.PointZ
		started.TargetX = &x
		started.TargetZ = &z
	}

	for _, s := range r.sessions.List() {
		if s.EntityID != actorID && (r.replication == nil || !r.replication.Knows(s.ID, actorID)) {
			continue
		}
		envelope := protocol.Envelope{
			Delivery: protocol.DeliveryReliableOrdered,
			Sequence: s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message: started,
		}
		if err := s.Connection().TrySend(envelope); err != nil {
			report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID,Delivery:protocol.DeliveryReliableOrdered,MessageType:protocol.MessageActionStarted,Err:err})
			if err == session.ErrConnectionClosed { continue }
		}
	}
}
