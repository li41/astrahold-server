package simulation

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestWorldTickMovesActorAndUpdatesAOI(t *testing.T) {
	move := movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 1)
	sim := New(spatial.NewGrid(10), move)
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer}, 10, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := sim.Spawn(world.EntityState{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 30}}}, 10, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if got := sim.QueryAOI(world.Position{}, 5, spatial.QueryOptions{}); len(got) != 1 {
		t.Fatalf("initial AOI count = %d", len(got))
	}
	if err := sim.SetMoveInput(2, movement.Input{Direction: world.Vec3{X: -1}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if errs := sim.Tick(1); len(errs) != 0 {
			t.Fatalf("tick errors = %v", errs)
		}
	}
	if got := sim.QueryAOI(world.Position{}, 5, spatial.QueryOptions{}); len(got) != 2 {
		t.Fatalf("AOI count after move = %d", len(got))
	}
}

func TestSetMoveInputDoesNotMoveUntilTickAndUpdatesFacing(t *testing.T) {
	move := movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 1)
	sim := New(spatial.NewGrid(10), move)
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer}, 5, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := sim.SetMoveInput(1, movement.Input{Direction: world.Vec3{Z: 1}}); err != nil {
		t.Fatal(err)
	}
	entity, _ := sim.Entity(1)
	if entity.Transform.Position.Z != 0 {
		t.Fatalf("position changed before tick: %+v", entity.Transform.Position)
	}
	if entity.Transform.Yaw < 89.99 || entity.Transform.Yaw > 90.01 {
		t.Fatalf("yaw=%f want=90", entity.Transform.Yaw)
	}

	if err := sim.SetMoveInput(1, movement.Input{}); err != nil {
		t.Fatal(err)
	}
	entity, _ = sim.Entity(1)
	if entity.Transform.Yaw < 89.99 || entity.Transform.Yaw > 90.01 {
		t.Fatalf("zero input changed facing yaw=%f", entity.Transform.Yaw)
	}
}
