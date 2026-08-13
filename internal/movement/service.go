// Package movement 實作伺服器權威移動。
package movement

import (
	"errors"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidDelta = errors.New("movement: invalid delta time")
	ErrStaleInput   = errors.New("movement: stale input sequence")
)

// Input 是 client 傳來的移動意圖，而不是 client 宣告的最終座標。
type Input struct {
	Sequence     uint32
	Direction    world.Vec3
	DeltaSeconds float32
}

// AgentState 是 movement subsystem 需要的權威角色狀態。
type AgentState struct {
	Position      world.Position
	Speed         float32
	Radius        float32
	MaxStepHeight float32
	LastSequence  uint32
}

// Service 套用移動輸入並交給 Navigator 驗證。
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

// ApplyInput 消耗一筆 input。
//
// client 只能提供方向與時間片；速度、碰撞與最終 Position 都由 server 決定。
func (s *Service) ApplyInput(state *AgentState, input Input) (world.Position, error) {
	if input.Sequence <= state.LastSequence {
		return state.Position, ErrStaleInput
	}
	if input.DeltaSeconds <= 0 {
		return state.Position, ErrInvalidDelta
	}

	// 即使被牆擋住，這個 sequence 仍視為已消耗，避免重送舊 input。
	state.LastSequence = input.Sequence

	delta := input.DeltaSeconds
	if delta > s.maxDeltaSeconds {
		delta = s.maxDeltaSeconds
	}

	direction := input.Direction.NormalizedXZ()
	displacement := direction.Scale(state.Speed * delta)
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
