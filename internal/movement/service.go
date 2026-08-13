// Package movement 只處理角色移動意圖與權威位移，不承擔網路 sequence/session 責任。
package movement

import (
	"errors"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidDelta = errors.New("movement: invalid server delta time")

type Input struct {
	Direction world.Vec3
}

type AgentState struct {
	Position      world.Position
	Speed         float32
	Radius        float32
	MaxStepHeight float32
	Direction     world.Vec3
}

type Service struct {
	navigator       navigation.Navigator
	maxDeltaSeconds float32
}

func NewService(navigator navigation.Navigator, maxDeltaSeconds float32) *Service {
	if navigator == nil {
		panic("movement: navigator is required")
	}
	if maxDeltaSeconds <= 0 {
		panic("movement: maxDeltaSeconds must be > 0")
	}
	return &Service{navigator: navigator, maxDeltaSeconds: maxDeltaSeconds}
}

func (s *Service) AcceptInput(state *AgentState, input Input) error {
	state.Direction = input.Direction.NormalizedXZ()
	return nil
}

func (s *Service) Step(state *AgentState, deltaSeconds float32) (world.Position, error) {
	if deltaSeconds <= 0 {
		return state.Position, ErrInvalidDelta
	}
	delta := deltaSeconds
	if delta > s.maxDeltaSeconds {
		delta = s.maxDeltaSeconds
	}
	displacement := state.Direction.Scale(state.Speed * delta)
	next, err := s.navigator.ResolveMove(state.Position, displacement, navigation.Agent{
		Radius:        state.Radius,
		MaxStepHeight: state.MaxStepHeight,
	})
	if err != nil {
		return state.Position, err
	}
	state.Position = next
	return next, nil
}
