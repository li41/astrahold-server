package siege

import (
	"errors"
	"math"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidThroneObjective = errors.New("siege: invalid throne objective")

type ObjectiveZone struct {
	Layer  world.LayerID
	Bounds gameplayworld.BoundsXZ
}

func (z ObjectiveZone) Valid() bool {
	return finite32(z.Bounds.MinX) && finite32(z.Bounds.MaxX) && finite32(z.Bounds.MinZ) && finite32(z.Bounds.MaxZ) &&
		z.Bounds.MinX < z.Bounds.MaxX && z.Bounds.MinZ < z.Bounds.MaxZ
}

func (z ObjectiveZone) Contains(position world.Position) bool {
	return z.Valid() && position.Layer == z.Layer && z.Bounds.Contains(position.X, position.Z)
}

type ThroneObjectiveDefinition struct {
	ID              string
	Zone            ObjectiveZone
	CaptureDuration time.Duration
}

// CaptureDuration==0 keeps older focused presence-only constructions valid. Production
// schema v3 requires a positive bounded duration before worldd starts.
func (d ThroneObjectiveDefinition) Valid() bool {
	return d.ID != "" && d.Zone.Valid() && d.CaptureDuration >= 0
}

type ParticipantPresence struct {
	EntityID world.EntityID
	Position world.Position
	Defeated bool
}

type ThronePresenceState struct {
	Revision        uint64
	ObjectiveID     string
	Active          bool
	AttackerCount   uint32
	DefenderCount   uint32
	Contested       bool
	CaptureEligible bool
}

type ThroneCaptureState struct {
	Revision           uint64
	ObjectiveID        string
	Active             bool
	Progress           time.Duration
	Required           time.Duration
	ReadyForResolution bool
}

type throneCaptureRuntime struct {
	state ThroneCaptureState
}

type throneRuntime struct {
	definition ThroneObjectiveDefinition
	state      ThronePresenceState
	capture    *throneCaptureRuntime
}

func newThroneRuntime(definition ThroneObjectiveDefinition) (*throneRuntime, error) {
	if !definition.Valid() {
		return nil, ErrInvalidThroneObjective
	}
	runtime := &throneRuntime{
		definition: definition,
		state: ThronePresenceState{
			Revision:    1,
			ObjectiveID: definition.ID,
		},
	}
	if definition.CaptureDuration > 0 {
		runtime.capture = &throneCaptureRuntime{state: ThroneCaptureState{
			Revision:    1,
			ObjectiveID: definition.ID,
			Required:    definition.CaptureDuration,
		}}
	}
	return runtime, nil
}

func (s *Service) ThronePresenceState() (ThronePresenceState, bool) {
	if s == nil || s.match == nil || s.match.throne == nil {
		return ThronePresenceState{}, false
	}
	return s.match.throne.state, true
}

func (s *Service) ThroneCaptureState() (ThroneCaptureState, bool) {
	if s == nil || s.match == nil || s.match.throne == nil || s.match.throne.capture == nil {
		return ThroneCaptureState{}, false
	}
	return s.match.throne.capture.state, true
}

// ObserveThronePresence consumes Server-owned entity positions, defeat state, and participant teams.
// It only derives occupancy/contest eligibility; capture progress and victory remain separate state.
func (s *Service) ObserveThronePresence(observations []ParticipantPresence) bool {
	if s == nil || s.match == nil || s.match.throne == nil {
		return false
	}

	current := s.match.throne.state
	next := ThronePresenceState{
		Revision:    current.Revision,
		ObjectiveID: s.match.throne.definition.ID,
		Active:      s.match.state.Phase == MatchPhaseThrone,
	}
	if next.Active {
		seen := make(map[world.EntityID]struct{}, len(observations))
		for _, observation := range observations {
			if observation.EntityID == 0 {
				continue
			}
			if _, duplicate := seen[observation.EntityID]; duplicate {
				continue
			}
			seen[observation.EntityID] = struct{}{}
			if observation.Defeated || !s.match.throne.definition.Zone.Contains(observation.Position) {
				continue
			}
			team, assigned := s.match.participantTeams[observation.EntityID]
			if !assigned {
				continue
			}
			switch team {
			case TeamAttacker:
				next.AttackerCount++
			case TeamDefender:
				next.DefenderCount++
			}
		}
	}
	next.Contested = next.Active && next.AttackerCount > 0 && next.DefenderCount > 0
	next.CaptureEligible = next.Active && next.AttackerCount > 0 && next.DefenderCount == 0

	if sameThronePresence(current, next) {
		return false
	}
	next.Revision = current.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	s.match.throne.state = next
	return true
}

// AdvanceThroneCapture applies Server-clock delta only while D.2A presence says capture is
// eligible. Any interruption before completion resets progress, requiring one continuous hold.
// Reaching Required latches ReadyForResolution but does not decide winner or ownership.
func (s *Service) AdvanceThroneCapture(delta time.Duration) bool {
	if s == nil || s.match == nil || s.match.throne == nil || s.match.throne.capture == nil {
		return false
	}
	current := s.match.throne.capture.state
	next := current
	next.Active = s.match.state.Phase == MatchPhaseThrone

	if current.ReadyForResolution {
		next.Progress = next.Required
	} else if !next.Active || !s.match.throne.state.CaptureEligible {
		next.Progress = 0
	} else if delta > 0 {
		remaining := next.Required - next.Progress
		if remaining <= 0 || delta >= remaining {
			next.Progress = next.Required
			next.ReadyForResolution = true
		} else {
			next.Progress += delta
		}
	}

	if sameThroneCapture(current, next) {
		return false
	}
	next.Revision = current.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	s.match.throne.capture.state = next
	return true
}

func sameThronePresence(a, b ThronePresenceState) bool {
	return a.ObjectiveID == b.ObjectiveID &&
		a.Active == b.Active &&
		a.AttackerCount == b.AttackerCount &&
		a.DefenderCount == b.DefenderCount &&
		a.Contested == b.Contested &&
		a.CaptureEligible == b.CaptureEligible
}

func sameThroneCapture(a, b ThroneCaptureState) bool {
	return a.ObjectiveID == b.ObjectiveID &&
		a.Active == b.Active &&
		a.Progress == b.Progress &&
		a.Required == b.Required &&
		a.ReadyForResolution == b.ReadyForResolution
}

func finite32(value float32) bool {
	f := float64(value)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
