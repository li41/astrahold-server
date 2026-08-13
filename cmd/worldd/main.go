package main

import (
	"log"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func main() {
	nav := navigation.Plane{
		MinX: -512, MaxX: 512,
		MinZ: -512, MaxZ: 512,
		Height: 0,
		Layer:  0,
	}
	move := movement.NewService(nav, 0.1)
	sim := simulation.New(spatial.NewGrid(32), move)

	player := world.EntityState{
		ID:   1,
		Kind: world.EntityPlayer,
		Transform: world.Transform{
			Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0},
		},
	}
	if err := sim.Spawn(player, 6, 0.35, 0.5); err != nil {
		log.Fatal(err)
	}
	if err := sim.SetMoveInput(1, movement.Input{Sequence: 1, Direction: world.Vec3{X: 1}}); err != nil {
		log.Fatal(err)
	}
	if errs := sim.Tick(0.05); len(errs) != 0 {
		log.Fatalf("world tick failed: %v", errs)
	}

	entity, _ := sim.Entity(1)
	log.Printf("Astrahold world core ready: entities=%d player=(%.2f, %.2f, %.2f)",
		len(sim.Snapshot()), entity.Transform.Position.X, entity.Transform.Position.Y, entity.Transform.Position.Z)
}
