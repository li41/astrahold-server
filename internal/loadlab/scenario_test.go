package loadlab

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestVerticalSiegePlayerFactoryUsesAllLayers(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewPlayerFactory(loaded.Definition, ScenarioVerticalSiege, 100)
	if err != nil {
		t.Fatal(err)
	}

	ground := factory(session.ID(1), world.EntityID(1))
	if ground.Entity.Transform.Position.Layer != 0 {
		t.Fatalf("ground layer = %d, want 0", ground.Entity.Transform.Position.Layer)
	}
	ramp := factory(session.ID(6), world.EntityID(6))
	if ramp.Entity.Transform.Position.Layer != 1 {
		t.Fatalf("ramp layer = %d, want 1", ramp.Entity.Transform.Position.Layer)
	}
	wall := factory(session.ID(8), world.EntityID(8))
	if wall.Entity.Transform.Position.Layer != 2 || wall.Entity.Transform.Position.Y != 8 {
		t.Fatalf("wall position = %+v, want layer=2 y=8", wall.Entity.Transform.Position)
	}
}

func TestMovementDirectionIsDeterministic(t *testing.T) {
	dx1, dz1 := MovementDirection(ScenarioDistributed, world.EntityID(42), 3*time.Second)
	dx2, dz2 := MovementDirection(ScenarioDistributed, world.EntityID(42), 3*time.Second)
	if dx1 != dx2 || dz1 != dz2 {
		t.Fatalf("direction is not deterministic: (%f,%f) != (%f,%f)", dx1, dz1, dx2, dz2)
	}

	dx, dz := MovementDirection(ScenarioGateZerg, world.EntityID(1), 0)
	if dx != 0 || dz != 1 {
		t.Fatalf("gate-zerg direction = (%f,%f), want (0,1)", dx, dz)
	}
}
