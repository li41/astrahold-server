package simulation

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestReplicationFrameUsesStableOrderAndGlobalTransformGeneration(t *testing.T) {
	move := movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 1)
	sim := New(spatial.NewGrid(32), move)
	if err := sim.Spawn(world.EntityState{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2}}}, 5, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1}}}, 5, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	builder := NewReplicationFrameBuilder()
	first := builder.Build(sim, 10)
	if len(first.Entities) != 2 || first.Entities[0].ID != 1 || first.Entities[1].ID != 2 {
		t.Fatalf("unstable frame order: %+v", first.Entities)
	}
	g1 := first.TransformGenerations[0]
	g2 := first.TransformGenerations[1]
	if g1 == 0 || g2 == 0 || g1 == g2 {
		t.Fatalf("unexpected initial generations: %v", first.TransformGenerations)
	}

	second := builder.Build(sim, 12)
	if second.TransformGenerations[0] != g1 || second.TransformGenerations[1] != g2 {
		t.Fatalf("unchanged transforms advanced generation: %v", second.TransformGenerations)
	}

	if err := sim.SetMoveInput(2, movement.Input{Direction: world.Vec3{X: 1}}); err != nil {
		t.Fatal(err)
	}
	if errs := sim.Tick(0.1); len(errs) != 0 {
		t.Fatalf("tick errors: %v", errs)
	}
	third := builder.Build(sim, 14)
	if third.TransformGenerations[0] != g1 {
		t.Fatalf("stationary entity generation changed: %d -> %d", g1, third.TransformGenerations[0])
	}
	if third.TransformGenerations[1] == g2 {
		t.Fatalf("moved entity generation did not advance: %d", g2)
	}
}
