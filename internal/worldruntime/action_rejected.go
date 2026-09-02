package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/world"
)

// rejectClientAction records the existing diagnostic rejection and, for a real client command,
// returns one explicit Server-authored reason to the source Session. The client action sequence is
// correlation only; it never participates in gameplay legality.
func (r *Runtime) rejectClientAction(
	name string,
	sourceSessionID session.ID,
	clientActionSequence uint32,
	actorID world.EntityID,
	actionID string,
	targetKind protocol.ActionTargetKind,
	err error,
	tick uint64,
	report *StepReport,
) {
	report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: sourceSessionID, Err: err})
	if clientActionSequence == 0 || sourceSessionID == 0 {
		return
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok {
		return
	}
	readyTick := uint64(0)
	reason := actionRejectionReason(err)
	if reason == protocol.ActionRejectionCooldown && r.combat != nil {
		readyTick = r.combat.ActionCooldownReadyTick(actorID, actionID)
	}
	message := protocol.ActionRejected{
		ClientActionSequence: clientActionSequence,
		ActorEntityID:        actorID,
		ActionID:             actionID,
		TargetKind:           targetKind,
		Reason:               reason,
		CooldownReadyTick:    readyTick,
	}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: tick,
		Message:    message,
	}
	if sendErr := s.Connection().TrySend(envelope); sendErr != nil {
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
			SessionID: sourceSessionID,
			Delivery: protocol.DeliveryReliableOrdered,
			MessageType: protocol.MessageActionRejected,
			Err: sendErr,
		})
	}
}

func actionRejectionReason(err error) protocol.ActionRejectionReason {
	switch {
	case errors.Is(err, combat.ErrActionCooldown), errors.Is(err, siege.ErrGateAttackCooldown):
		return protocol.ActionRejectionCooldown
	case errors.Is(err, character.ErrInsufficientResource):
		return protocol.ActionRejectionInsufficientResource
	case errors.Is(err, ErrEntityOutOfRange), errors.Is(err, ErrPointOutOfRange), errors.Is(err, siege.ErrGateOutOfRange):
		return protocol.ActionRejectionOutOfRange
	case errors.Is(err, ErrEntityWrongLayer), errors.Is(err, siege.ErrGateWrongLayer):
		return protocol.ActionRejectionWrongLayer
	case errors.Is(err, ErrEntityNoLineOfSight), errors.Is(err, ErrPointNoLineOfSight), errors.Is(err, siege.ErrGateNoLineOfSight):
		return protocol.ActionRejectionLineOfSight
	case errors.Is(err, character.ErrCharacterDefeated):
		return protocol.ActionRejectionDefeated
	case errors.Is(err, ErrEntityReviveProtected):
		return protocol.ActionRejectionReviveProtected
	case errors.Is(err, combat.ErrUnknownAction):
		return protocol.ActionRejectionUnknownAction
	case errors.Is(err, combat.ErrTargetNotAllowed),
		errors.Is(err, ErrInvalidEntityTarget),
		errors.Is(err, ErrSelfTarget),
		errors.Is(err, ErrResurrectionTargetNotPlayer),
		errors.Is(err, character.ErrCharacterNotDefeated),
		errors.Is(err, siege.ErrUnknownGate),
		errors.Is(err, siege.ErrGateDestroyed),
		errors.Is(err, siege.ErrGateBlockerDisabled):
		return protocol.ActionRejectionInvalidTarget
	default:
		return protocol.ActionRejectionServerRejected
	}
}
