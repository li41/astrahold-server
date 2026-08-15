package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

type siegeDeliveryStamp struct {
	Revision uint64
	Team     protocol.SiegeTeam
}

func (r *Runtime) replicateSiegeState(tick uint64, report *StepReport, sessions []*session.Session) {
	if r.siege == nil {
		return
	}
	state, ok := r.siege.MatchState()
	if !ok {
		r.resetSiegeCompletionScheduling()
		return
	}

	active := make(map[session.ID]struct{}, len(sessions))
	pendingDeliveries := 0
	for _, s := range sessions {
		active[s.ID] = struct{}{}
		team := protocol.SiegeTeamUnknown
		if domainTeam, assigned := r.siege.ParticipantTeam(s.EntityID); assigned {
			team = protocolSiegeTeam(domainTeam)
		}
		stamp := r.sessionSiegeState[s.ID]
		if stamp.Revision == state.Revision && stamp.Team == team {
			continue
		}

		message := protocol.SiegeMatchState{
			Revision:          state.Revision,
			MatchID:           state.MatchID,
			AttackerID:        state.AttackerID,
			DefenderID:        state.DefenderID,
			YourTeam:          team,
			Phase:             protocolSiegePhase(state.Phase),
			BreachGateID:      state.BreachGateID,
			ThroneObjectiveID: state.ThroneObjectiveID,
			GateBreached:      state.GateBreached,
		}
		envelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    message,
		}
		if err := s.Connection().TrySend(envelope); err != nil {
			pendingDeliveries++
			report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: envelope.Delivery, MessageType: message.Type(), Err: err})
			continue
		}
		r.sessionSiegeState[s.ID] = siegeDeliveryStamp{Revision: state.Revision, Team: team}
	}

	for id := range r.sessionSiegeState {
		if _, ok := active[id]; !ok {
			delete(r.sessionSiegeState, id)
		}
	}

	// D.3D consumes the delivery result from this same per-session pass. There is no second
	// hot-loop session scan and no claim of Client acknowledgement: a current stamp only means
	// the Reliable TrySend accepted this authoritative revision into that session's outbound path.
	r.observeSiegeCompletionScheduling(state, len(sessions), pendingDeliveries, report)
}

func protocolSiegeTeam(team siege.Team) protocol.SiegeTeam {
	switch team {
	case siege.TeamAttacker:
		return protocol.SiegeTeamAttacker
	case siege.TeamDefender:
		return protocol.SiegeTeamDefender
	default:
		return protocol.SiegeTeamUnknown
	}
}

func protocolSiegePhase(phase siege.MatchPhase) protocol.SiegePhase {
	switch phase {
	case siege.MatchPhaseGate:
		return protocol.SiegePhaseGate
	case siege.MatchPhaseThrone:
		return protocol.SiegePhaseThrone
	case siege.MatchPhaseCompleted:
		return protocol.SiegePhaseCompleted
	default:
		return protocol.SiegePhaseUnknown
	}
}
