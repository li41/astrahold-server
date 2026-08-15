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
	ErrRoundResetUnavailable       = errors.New("siege: round reset unavailable")
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
	Round             uint64
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
			Round:             1,
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

// ConfigureCastleOwnershipPersistence restores durable ownership before gameplay starts,
// aligns the fresh Gate round so the durable owner is the defender, and installs the commit
// barrier used by throne resolution. The ownership revision is the durable round epoch.
func (s *Service) ConfigureCastleOwnershipPersistence(state CastleOwnershipState, committer CastleOwnershipCommitter) error {
	if s == nil || s.match == nil {
		return ErrMatchUnavailable
	}
	if s.match.state.Phase != MatchPhaseGate || s.match.state.GateBreached || s.match.state.WinnerTeam != TeamUnknown || s.match.state.WinnerID != "" {
		return ErrInvalidCastleOwnership
	}
	if !validCastleOwnership(state) || !s.validSideID(state.OwnerID) || committer == nil {
		return ErrInvalidCastleOwnership
	}
	attackerID, defenderID, ok := s.rolesForOwner(state.OwnerID)
	if !ok {
		return ErrInvalidCastleOwnership
	}
	s.match.ownership = state
	s.match.ownershipCommitter = committer
	s.match.state.Round = state.Revision
	s.applyRoles(attackerID, defenderID)
	return nil
}

// AssignResolvedParticipant maps only an already-trusted CharacterID through this match's
// Server-owned roster. D.3C keeps the two-side roster aligned whenever round roles rotate.
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
	s.bumpMatchRevision()
	return true
}

// PrepareThroneCaptureResolution exposes a deterministic CAS intent but does not mutate match
// or ownership state. The current round attacker, not the startup config role, becomes owner.
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
		OwnerID:          s.match.state.AttackerID,
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
	nextMatch.WinnerID = s.match.state.AttackerID
	s.match.state = nextMatch
	s.bumpMatchRevision()
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

// ResolveThroneCapture preserves the bool-only call site used by focused tests. Production
// worldd configures a durable committer; persistence failure remains a no-op here.
func (s *Service) ResolveThroneCapture() bool {
	resolved, _ := s.ResolveThroneCaptureWithError()
	return resolved
}

// StartNextRound resets the completed Siege back to Gate phase. It restores the configured
// breach gate before publishing the new round, derives defender from durable castle ownership,
// derives attacker from the other configured side, clears winner/objective state, and advances
// the round epoch to the ownership revision. Scheduling this operation is deliberately external.
func (s *Service) StartNextRound(scene World) (bool, error) {
	if s == nil || s.match == nil {
		return false, ErrMatchUnavailable
	}
	if s.match.state.Phase != MatchPhaseCompleted {
		return false, nil
	}
	if scene == nil || !validCastleOwnership(s.match.ownership) || !s.validSideID(s.match.ownership.OwnerID) {
		return false, ErrRoundResetUnavailable
	}
	if s.match.state.Round == ^uint64(0) || s.match.ownership.Revision != s.match.state.Round+1 {
		return false, ErrRoundResetUnavailable
	}
	attackerID, defenderID, ok := s.rolesForOwner(s.match.ownership.OwnerID)
	if !ok || s.match.ownership.OwnerID != s.match.state.WinnerID {
		return false, ErrRoundResetUnavailable
	}
	gate := s.gates[s.match.definition.BreachGateID]
	if gate == nil || gate.definition.BlockerID == "" {
		return false, ErrRoundResetUnavailable
	}
	enabled, err := scene.BlockerEnabled(gate.definition.BlockerID)
	if err != nil {
		return false, err
	}
	if !enabled {
		if err := scene.SetBlockerEnabled(gate.definition.BlockerID, true); err != nil {
			return false, err
		}
	}

	gate.hp = gate.definition.MaxHP
	for key := range s.nextAttackTick {
		if key.gateID == gate.definition.ID {
			delete(s.nextAttackTick, key)
		}
	}
	s.resetThroneForNextRound()

	next := s.match.state
	next.Round = s.match.ownership.Revision
	next.Phase = MatchPhaseGate
	next.GateBreached = false
	next.WinnerTeam = TeamUnknown
	next.WinnerID = ""
	s.match.state = next
	s.applyRoles(attackerID, defenderID)
	s.bumpMatchRevision()
	return true, nil
}

func (s *Service) resetThroneForNextRound() {
	if s.match == nil || s.match.throne == nil {
		return
	}
	presence := s.match.throne.state
	nextPresence := ThronePresenceState{Revision: presence.Revision, ObjectiveID: s.match.throne.definition.ID}
	if !sameThronePresence(presence, nextPresence) {
		nextPresence.Revision = nextRevision(presence.Revision)
		s.match.throne.state = nextPresence
	}
	if s.match.throne.capture == nil {
		return
	}
	capture := s.match.throne.capture.state
	nextCapture := ThroneCaptureState{
		Revision:    capture.Revision,
		ObjectiveID: s.match.throne.definition.ID,
		Required:    s.match.throne.definition.CaptureDuration,
	}
	if !sameThroneCapture(capture, nextCapture) {
		nextCapture.Revision = nextRevision(capture.Revision)
		s.match.throne.capture.state = nextCapture
	}
}

func (s *Service) validSideID(sideID string) bool {
	return s != nil && s.match != nil && (sideID == s.match.definition.AttackerID || sideID == s.match.definition.DefenderID)
}

func (s *Service) rolesForOwner(ownerID string) (attackerID, defenderID string, ok bool) {
	if s == nil || s.match == nil {
		return "", "", false
	}
	switch ownerID {
	case s.match.definition.DefenderID:
		return s.match.definition.AttackerID, s.match.definition.DefenderID, true
	case s.match.definition.AttackerID:
		return s.match.definition.DefenderID, s.match.definition.AttackerID, true
	default:
		return "", "", false
	}
}

func (s *Service) applyRoles(attackerID, defenderID string) {
	if s.match.state.AttackerID != attackerID {
		for id, team := range s.match.trustedParticipantTeams {
			s.match.trustedParticipantTeams[id] = oppositeTeam(team)
		}
		for entityID, team := range s.match.participantTeams {
			s.match.participantTeams[entityID] = oppositeTeam(team)
		}
	}
	s.match.state.AttackerID = attackerID
	s.match.state.DefenderID = defenderID
}

func oppositeTeam(team Team) Team {
	switch team {
	case TeamAttacker:
		return TeamDefender
	case TeamDefender:
		return TeamAttacker
	default:
		return team
	}
}

func (s *Service) bumpMatchRevision() {
	if s == nil || s.match == nil {
		return
	}
	s.match.state.Revision = nextRevision(s.match.state.Revision)
}

func nextRevision(current uint64) uint64 {
	next := current + 1
	if next == 0 {
		return 1
	}
	return next
}

func validCastleOwnership(state CastleOwnershipState) bool {
	if state.Revision == 0 || state.OwnerID == "" {
		return false
	}
	return (state.PreviousOwnerID == "") == (state.LastTransferMatchID == "")
}
