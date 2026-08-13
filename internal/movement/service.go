// Package movement 實作伺服器權威移動。
package movement

import (
	"errors"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidDelta = errors.New("movement: invalid server delta time")
	ErrStaleInput   = errors.New("movement: stale input sequence")
)

// Input 是 client 傳來的移動意圖，而不是 client 宣告的座標或時間片。
// 真正位移距離只由 server tick 與 server-side speed 決定。
type Input struct {
	Sequence  uint32
	Direction world.Vec3
}

// AgentState 是 movement subsystem 需要的權威角色狀態。
type AgentState struct {
	Position      world.Position
	Speed         float32
	Radius        float32
	MaxStepHeight float32
	LastSequence  uint32
	Direction     world.Vec3
}

// Service 接收 client 意圖，並在 server tick 中透過 Navigator 推進權威位置。
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

// AcceptInput 只更新移動意圖，不直接改變角色位置。
func (s *Service) AcceptInput(state *AgentState, input Input) error {
	if input.Sequence <= state.LastSequence {
		return ErrStaleInput
	}
	state.LastSequence = input.Sequence
	state.Direction = input.Direction.NormalizedXZ()
	return nil
}

// Step 由 server simulation tick 呼叫。
// deltaSeconds 必須來自 server clock，而不是 client packet。
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
