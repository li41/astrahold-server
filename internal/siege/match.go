package siege

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrMatchUnavailable       = errors.New("siege: match unavailable")
	ErrInvalidMatchDefinition = errors.New("siege: invalid match definition")
	ErrInvalidParticipant     = errors.New("siege: invalid participant")
)

type Team uint8

const (
	TeamUnknown Team = iota
	TeamAttacker
	TeamDefender
)

func (t Team) Valid() bool { return t == TeamAttacker || t == TeamDefender }

type MatchPhase uint8

const (
	MatchPhaseUnknown MatchPhase = iota
	MatchPhaseGate
	MatchPhaseThrone
	MatchPhaseCompleted
)

type MatchDefinition struct {
	ID                string
	AttackerID        string
	DefenderID        string
	BreachGateID      string
	ThroneObjectiveID string
	Throne            *ThroneObjectiveDefinition
}

func (d MatchDefinition) Valid() bool {
	if d.ID == "" || d.AttackerID == "" || d.DefenderID == "" || d.AttackerID == d.DefenderID || d.BreachGateID == "" || d.ThroneObjectiveID == "" {
		return false
	}
	return d.Throne == nil || (d.Throne.Valid() && d.Throne.ID == d.ThroneObjectiveID)
}

type MatchState struct {
	Revision          uint64
	MatchID           string
	AttackerID        string
	DefenderID        string
	Phase             MatchPhase
	BreachGateID      string
	ThroneObjectiveID string
	GateBreached      bool
}

type matchRuntime struct {
	definition       MatchDefinition
	state            MatchState
	participantTeams map[world.EntityID]Team
	throne           *throneRuntime
}

func (s *Service) ConfigureMatch(definition MatchDefinition) error {
	if s == nil {
		return ErrMatchUnavailable
	}
	if !definition.Valid() {
		return ErrInvalidMatchDefinition
	}
	if _, ok := s.gates[definition.BreachGateID]; !ok {
		return fmt.Errorf("%w: breach gate %q", ErrInvalidMatchDefinition, definition.BreachGateID)
	}
	var throne *throneRuntime
	if definition.Throne != nil {
		configured, err := newThroneRuntime(*definition.Throne)
		if err != nil {
			return err
		}
		throne = configured
	}
	s.match = &matchRuntime{
		definition: definition,
		state: MatchState{
			Revision:          1,
			MatchID:           definition.ID,
			AttackerID:        definition.AttackerID,
			DefenderID:        definition.DefenderID,
			Phase:             MatchPhaseGate,
			BreachGateID:      definition.BreachGateID,
			ThroneObjectiveID: definition.ThroneObjectiveID,
		},
		participantTeams: make(map[world.EntityID]Team),
		throne:           throne,
	}
	return nil
}

func (s *Service) MatchState() (MatchState, bool) {
	if s == nil || s.match == nil {
		return MatchState{}, false
	}
	return s.match.state, true
}

func (s *Service) AssignParticipant(entityID world.EntityID, team Team) error {
	if s == nil || s.match == nil {
		return ErrMatchUnavailable
	}
	if entityID == 0 || !team.Valid() {
		return ErrInvalidParticipant
	}
	s.match.participantTeams[entityID] = team
	return nil
}

func (s *Service) ParticipantTeam(entityID world.EntityID) (Team, bool) {
	if s == nil || s.match == nil || entityID == 0 {
		return TeamUnknown, false
	}
	team, ok := s.match.participantTeams[entityID]
	return team, ok
}

// ObserveGateState consumes already-authoritative Gate state. It never derives breach truth
// from Client intent or presentation. A configured breach gate advances Gate -> Throne once.
func (s *Service) ObserveGateState(state GateState) bool {
	if s == nil || s.match == nil || state.ID != s.match.definition.BreachGateID || !state.Destroyed || s.match.state.Phase != MatchPhaseGate {
		return false
	}
	s.match.state.GateBreached = true
	s.match.state.Phase = MatchPhaseThrone
	s.match.state.Revision++
	if s.match.state.Revision == 0 {
		s.match.state.Revision = 1
	}
	return true
}
