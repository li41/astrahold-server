package movement

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestApplyInputIsServerAuthoritative(t *testing.T) {
	service := NewService(navigation.Plane{
		MinX: -10, MaxX: 10,
		MinZ: -10, MaxZ: 10,
		Height: 3,
		Layer:  2,
	}, 0.1)

	state := AgentState{
		Position: world.Position{Layer: 2},
		Speed:    5,
		Radius:   0.35,
	}

	position, err := service.ApplyInput(&state, Input{
		Sequence:     1,
		Direction:    world.Vec3{X: 100, Y: 999, Z: 0},
		DeltaSeconds: 1, // 會被 server clamp 成 0.1 秒。
	})
	if err != nil {
		t.Fatal(err)
	}
	if position.X != 0.5 || position.Y != 3 || position.Z != 0 {
		t.Fatalf("position = %+v, want X=0.5 Y=3 Z=0", position)
	}
}

func TestApplyInputRejectsStaleSequence(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}, 0.1)
	state := AgentState{Speed: 5}

	_, err := service.ApplyInput(&state, Input{Sequence: 1, Direction: world.Vec3{X: 1}, DeltaSeconds: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ApplyInput(&state, Input{Sequence: 1, Direction: world.Vec3{X: 1}, DeltaSeconds: 0.1})
	if !errors.Is(err, ErrStaleInput) {
		t.Fatalf("err = %v, want ErrStaleInput", err)
	}
}

func TestApplyInputConsumesBlockedSequence(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1}, 1)
	state := AgentState{Position: world.Position{X: 0.9}, Speed: 5}

	_, err := service.ApplyInput(&state, Input{Sequence: 10, Direction: world.Vec3{X: 1}, DeltaSeconds: 1})
	if !errors.Is(err, navigation.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	if state.LastSequence != 10 {
		t.Fatalf("LastSequence = %d, want 10", state.LastSequence)
	}
}
