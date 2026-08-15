package siege

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrMatchUnavailable            = errors.New("siege: match unavailable")
	ErrInvalidMatchDefinition      = errors.New("siege: invalid match definition")
	ErrInvalidParticipant          = errors.New("siege: invalid participant")
	ErrInvalidCastleOwnership      = errors.New("siege: invalid castle ownership")
	ErrInvalidCastleTransfer       = errors.New("siege: invalid castle ownership transfer")
	ErrThroneResolutionUnavailable = errors.New("siege: throne resolution unavailable")
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
	ParticipantTeams  map[characteridentity.ID]Team
}

func (d MatchDefinition) Valid() bool {
	if d.ID == "" || d.AttackerID == "" || d.DefenderID == "" || d.AttackerID == d.DefenderID || d.BreachGateID == "" || d.ThroneObjectiveID == "" {
		return false
	}
	if d.Throne != nil && (!d.Throne.Valid() || d.Throne.ID != d.ThroneObjectiveID) {
		return false
	}
	for characterID, team := range d.ParticipantTeams {
		binding, err := characteridentity.NewTrusted(string(characterID))
		if err != nil || binding.ID != characterID || !team.Valid() {
			return false
		}
	}
	return true
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
	WinnerTeam        Team
	WinnerID          string
}

// CastleOwnershipState is the Server-authoritative owner snapshot for the castle defended by
// this match. D.3B may restore it from durable single-writer storage before the world loop starts.
type CastleOwnershipState struct {
	Revision            uint64
	OwnerID             string
	PreviousOwnerID     string
	LastTransferMatchID string
}

// CastleOwnershipTransfer is the compare-and-swap intent emitted by a ready throne capture.
// ExpectedRevision and PreviousOwnerID fence stale storage writers; MatchID is provenance.
type CastleOwnershipTransfer struct {
	ExpectedRevision uint64
	PreviousOwnerID  string
	OwnerID          string
	MatchID          string
}

func (t CastleOwnershipTransfer) Valid() bool {
	return t.ExpectedRevision > 0 && t.PreviousOwnerID != "" && t.OwnerID != "" && t.MatchID != ""
}

func (t CastleOwnershipTransfer) RequiresOwnershipChange() bool {
	return t.Valid() && t.PreviousOwnerID != t.OwnerID
}

// CastleOwnershipCommitter durably applies exactly one ownership transfer and returns the
// committed authoritative snapshot. The production worldd implementation performs fsync + CAS.
type CastleOwnershipCommitter func(CastleOwnershipTransfer) (CastleOwnershipState, error)

type matchRuntime struct {
	definition              MatchDefinition
	state                   MatchState
	ownership               CastleOwnershipState
	ownershipCommitter      CastleOwnershipCommitter
	participantTeams        map[world.EntityID]Team
	trustedParticipantTeams map[characteridentity.ID]Team
	throne                  *throneRuntime
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
	trustedTeams := make(map[characteridentity.ID]Team, len(definition.ParticipantTeams))
	for characterID, team := range definition.ParticipantTeams {
		trustedTeams[characterID] = team
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
		ownership: CastleOwnershipState{
			Revision: 1,
			OwnerID:  definition.DefenderID,
		},
		participantTeams:        make(map[world.EntityID]Team),
		trustedParticipantTeams: trustedTeams,
		throne:                  throne,
	}
	return nil
}

func (s *Service) MatchState() (MatchState, bool) {
	if s == nil || s.match == nil {
		return MatchState{}, false
	}
	return s.match.state, true
}

func (s *Service) CastleOwnershipState() (CastleOwnershipState, bool) {
	if s == nil || s.match == nil {
		return CastleOwnershipState{}, false
	}
	return s.match.ownership, true
}

// ConfigureCastleOwnershipPersistence restores the durable owner before gameplay starts and
// installs the commit barrier used by throne resolution. It is intentionally startup-only.
func (s *Service) ConfigureCastleOwnershipPersistence(state CastleOwnershipState, committer CastleOwnershipCommitter) error {
	if s == nil || s.match == nil {
		return ErrMatchUnavailable
	}
	if s.match.state.Phase != MatchPhaseGate || s.match.state.GateBreached || s.match.state.WinnerTeam != TeamUnknown || s.match.state.WinnerID != "" {
		return ErrInvalidCastleOwnership
	}
	if !validCastleOwnership(state) || (state.OwnerID != s.match.definition.AttackerID && state.OwnerID != s.match.definition.DefenderID) || committer == nil {
		return ErrInvalidCastleOwnership
	}
	s.match.ownership = state
	s.match.ownershipCommitter = committer
	return nil
}

// AssignResolvedParticipant maps only an already-trusted CharacterID through this match's
// Server-owned roster. Ephemeral and unlisted trusted identities remain unknown.
func (s *Service) AssignResolvedParticipant(entityID world.EntityID, identity characteridentity.Binding) (bool, error) {
	if s == nil || s.match == nil {
		return false, ErrMatchUnavailable
	}
	if entityID == 0 || !identity.Valid() {
		return false, ErrInvalidParticipant
	}
	if identity.Assurance != characteridentity.AssuranceTrusted {
		return false, nil
	}
	team, ok := s.match.trustedParticipantTeams[identity.ID]
	if !ok {
		return false, nil
	}
	if err := s.AssignParticipant(entityID, team); err != nil {
		return false, err
	}
	return true, nil
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

func (s *Service) RemoveParticipant(entityID world.EntityID) {
	if s == nil || s.match == nil || entityID == 0 {
		return
	}
	delete(s.match.participantTeams, entityID)
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

// PrepareThroneCaptureResolution exposes a deterministic CAS intent but does not mutate match
// or ownership state. Persistence may fail/retry while the D.2B readiness latch remains set.
func (s *Service) PrepareThroneCaptureResolution() (CastleOwnershipTransfer, bool) {
	if s == nil || s.match == nil || s.match.throne == nil || s.match.throne.capture == nil {
		return CastleOwnershipTransfer{}, false
	}
	if s.match.state.Phase != MatchPhaseThrone || s.match.state.WinnerTeam != TeamUnknown || s.match.state.WinnerID != "" {
		return CastleOwnershipTransfer{}, false
	}
	if !s.match.throne.capture.state.ReadyForResolution || !validCastleOwnership(s.match.ownership) {
		return CastleOwnershipTransfer{}, false
	}
	return CastleOwnershipTransfer{
		ExpectedRevision: s.match.ownership.Revision,
		PreviousOwnerID:  s.match.ownership.OwnerID,
		OwnerID:          s.match.definition.AttackerID,
		MatchID:          s.match.definition.ID,
	}, true
}

// CommitThroneCaptureResolution publishes a previously durable ownership snapshot and then
// completes the match. No match revision changes unless the ownership result validates against
// the current prepared transfer.
func (s *Service) CommitThroneCaptureResolution(ownership CastleOwnershipState) error {
	transfer, ok := s.PrepareThroneCaptureResolution()
	if !ok {
		return ErrThroneResolutionUnavailable
	}
	if !validCastleOwnership(ownership) {
		return ErrInvalidCastleOwnership
	}
	current := s.match.ownership
	if transfer.RequiresOwnershipChange() {
		if transfer.ExpectedRevision == ^uint64(0) || ownership.Revision != transfer.ExpectedRevision+1 || ownership.PreviousOwnerID != transfer.PreviousOwnerID || ownership.OwnerID != transfer.OwnerID || ownership.LastTransferMatchID != transfer.MatchID {
			return ErrInvalidCastleOwnership
		}
	} else if ownership != current {
		return ErrInvalidCastleOwnership
	}

	nextMatch := s.match.state
	nextMatch.Phase = MatchPhaseCompleted
	nextMatch.WinnerTeam = TeamAttacker
	nextMatch.WinnerID = s.match.definition.AttackerID
	nextMatch.Revision++
	if nextMatch.Revision == 0 {
		nextMatch.Revision = 1
	}

	s.match.state = nextMatch
	s.match.ownership = ownership
	s.ObserveThronePresence(nil)
	s.AdvanceThroneCapture(0)
	return nil
}

// ResolveThroneCaptureWithError enforces the D.3B durable completion barrier when configured.
// Storage failure leaves the match in Throne with ReadyForResolution latched so a later tick can retry.
func (s *Service) ResolveThroneCaptureWithError() (bool, error) {
	transfer, ok := s.PrepareThroneCaptureResolution()
	if !ok {
		return false, nil
	}
	current := s.match.ownership
	committed := current
	if transfer.RequiresOwnershipChange() {
		if s.match.ownershipCommitter != nil {
			var err error
			committed, err = s.match.ownershipCommitter(transfer)
			if err != nil {
				return false, err
			}
		} else {
			if transfer.ExpectedRevision == ^uint64(0) {
				return false, ErrInvalidCastleOwnership
			}
			committed = CastleOwnershipState{
				Revision:            transfer.ExpectedRevision + 1,
				OwnerID:             transfer.OwnerID,
				PreviousOwnerID:     transfer.PreviousOwnerID,
				LastTransferMatchID: transfer.MatchID,
			}
		}
	}
	if err := s.CommitThroneCaptureResolution(committed); err != nil {
		return false, err
	}
	return true, nil
}

// ResolveThroneCapture preserves the D.3A bool-only call site. Production worldd configures a
// committer whose failures are logged at the persistence boundary; failure remains a no-op here.
func (s *Service) ResolveThroneCapture() bool {
	resolved, _ := s.ResolveThroneCaptureWithError()
	return resolved
}

func validCastleOwnership(state CastleOwnershipState) bool {
	if state.Revision == 0 || state.OwnerID == "" {
		return false
	}
	return (state.PreviousOwnerID == "") == (state.LastTransferMatchID == "")
}
