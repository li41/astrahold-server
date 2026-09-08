package simulation

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSetFacingDirectionRotatesWithoutChangingMovement(t *testing.T) {
	move := movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 1)
	sim := New(spatial.NewGrid(10), move)
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityMonster}, 5, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := sim.SetMoveInput(1, movement.Input{}); err != nil {
		t.Fatal(err)
	}
	if err := sim.SetFacingDirection(1, world.Vec3{Z: 5}); err != nil {
		t.Fatal(err)
	}
	entity, _ := sim.Entity(1)
	if entity.Transform.Yaw < 89.99 || entity.Transform.Yaw > 90.01 {
		t.Fatalf("yaw=%f want=90", entity.Transform.Yaw)
	}
	if errs := sim.Tick(0.1); len(errs) != 0 {
		t.Fatalf("tick errors=%v", errs)
	}
	entity, _ = sim.Entity(1)
	if entity.Transform.Position.X != 0 || entity.Transform.Position.Z != 0 {
		t.Fatalf("facing-only update moved entity: %+v", entity.Transform.Position)
	}

	if err := sim.SetFacingDirection(1, world.Vec3{}); err != nil {
		t.Fatal(err)
	}
	entity, _ = sim.Entity(1)
	if entity.Transform.Yaw < 89.99 || entity.Transform.Yaw > 90.01 {
		t.Fatalf("zero facing direction changed yaw=%f", entity.Transform.Yaw)
	}
}
