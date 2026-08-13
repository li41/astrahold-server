package simulation

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestWorldMoveUpdatesAOI(t *testing.T) {
	move := movement.NewService(navigation.Plane{
		MinX: -100, MaxX: 100,
		MinZ: -100, MaxZ: 100,
	}, 1)
	sim := New(spatial.NewGrid(10), move)

	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{}},
	}, 10, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := sim.Spawn(world.EntityState{
		ID: 2, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 30}},
	}, 10, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	if got := sim.QueryAOI(world.Position{}, 5, spatial.QueryOptions{}); len(got) != 1 {
		t.Fatalf("initial AOI count = %d, want 1", len(got))
	}

	_, err := sim.ApplyMove(2, movement.Input{
		Sequence: 1,
		Direction: world.Vec3{X: -1},
		DeltaSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sim.ApplyMove(2, movement.Input{
		Sequence: 2,
		Direction: world.Vec3{X: -1},
		DeltaSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sim.ApplyMove(2, movement.Input{
		Sequence: 3,
		Direction: world.Vec3{X: -1},
		DeltaSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := sim.QueryAOI(world.Position{}, 5, spatial.QueryOptions{}); len(got) != 2 {
		t.Fatalf("AOI count after move = %d, want 2", len(got))
	}
}
