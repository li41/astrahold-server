package movement

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMovementUsesServerDelta(t *testing.T) {
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

	if err := service.AcceptInput(&state, Input{
		Sequence:  1,
		Direction: world.Vec3{X: 100, Y: 999, Z: 0},
	}); err != nil {
		t.Fatal(err)
	}

	position, err := service.Step(&state, 1) // server delta 仍會 clamp 成 0.1 秒。
	if err != nil {
		t.Fatal(err)
	}
	if position.X != 0.5 || position.Y != 3 || position.Z != 0 {
		t.Fatalf("position = %+v, want X=0.5 Y=3 Z=0", position)
	}
}

func TestAcceptInputRejectsStaleSequence(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}, 0.1)
	state := AgentState{Speed: 5}

	if err := service.AcceptInput(&state, Input{Sequence: 1, Direction: world.Vec3{X: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := service.AcceptInput(&state, Input{Sequence: 1, Direction: world.Vec3{X: 1}}); !errors.Is(err, ErrStaleInput) {
		t.Fatalf("err = %v, want ErrStaleInput", err)
	}
}

func TestBlockedMovementDoesNotChangePosition(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1}, 1)
	state := AgentState{Position: world.Position{X: 0.9}, Speed: 5}

	if err := service.AcceptInput(&state, Input{Sequence: 10, Direction: world.Vec3{X: 1}}); err != nil {
		t.Fatal(err)
	}
	position, err := service.Step(&state, 1)
	if !errors.Is(err, navigation.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	if position.X != 0.9 {
		t.Fatalf("position.X = %v, want 0.9", position.X)
	}
}
