package movement

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMovementUsesServerDelta(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Height: 3, Layer: 2}, 0.1)
	state := AgentState{Position: world.Position{Layer: 2}, Speed: 5, Radius: 0.35}
	if err := service.AcceptInput(&state, Input{Direction: world.Vec3{X: 100, Y: 999}}); err != nil {
		t.Fatal(err)
	}
	position, err := service.Step(&state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if position.X != 0.5 || position.Y != 3 || position.Z != 0 {
		t.Fatalf("position = %+v", position)
	}
}

func TestBlockedMovementDoesNotChangePosition(t *testing.T) {
	service := NewService(navigation.Plane{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1}, 1)
	state := AgentState{Position: world.Position{X: 0.9}, Speed: 5}
	if err := service.AcceptInput(&state, Input{Direction: world.Vec3{X: 1}}); err != nil {
		t.Fatal(err)
	}
	position, err := service.Step(&state, 1)
	if !errors.Is(err, navigation.ErrBlocked) {
		t.Fatalf("err = %v", err)
	}
	if position.X != 0.9 {
		t.Fatalf("position.X = %v", position.X)
	}
}
