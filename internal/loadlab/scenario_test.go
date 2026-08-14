package loadlab

import (
	"math"
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

func TestTeleportChurnSwapsHalfOfEachCluster(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	const clients = 100
	factory, err := NewPlayerFactory(loaded.Definition, ScenarioTeleportChurn, clients)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := TeleportChurnTargets(loaded.Definition, clients)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != clients/2 {
		t.Fatalf("targets=%d want=%d", len(targets), clients/2)
	}

	groupSize := clients / 2
	moversPerGroup := groupSize / 2
	for local := 0; local < moversPerGroup; local++ {
		westID := world.EntityID(local + 1)
		eastID := world.EntityID(groupSize + local + 1)
		westInitial := factory(session.ID(westID), westID).Entity.Transform.Position
		eastInitial := factory(session.ID(eastID), eastID).Entity.Transform.Position
		if targets[westID] != eastInitial {
			t.Fatalf("west mover %d target=%+v want east slot=%+v", westID, targets[westID], eastInitial)
		}
		if targets[eastID] != westInitial {
			t.Fatalf("east mover %d target=%+v want west slot=%+v", eastID, targets[eastID], westInitial)
		}
	}

	westStationaryID := world.EntityID(moversPerGroup + 1)
	eastStationaryID := world.EntityID(groupSize + moversPerGroup + 1)
	if _, ok := targets[westStationaryID]; ok {
		t.Fatalf("west stationary entity %d unexpectedly moved", westStationaryID)
	}
	if _, ok := targets[eastStationaryID]; ok {
		t.Fatalf("east stationary entity %d unexpectedly moved", eastStationaryID)
	}

	west := factory(1, 1).Entity.Transform.Position
	east := factory(session.ID(groupSize+1), world.EntityID(groupSize+1)).Entity.Transform.Position
	dx := float64(east.X - west.X)
	dz := float64(east.Z - west.Z)
	if math.Sqrt(dx*dx+dz*dz) <= 64 {
		t.Fatalf("cluster sample distance must exceed AOI radius: west=%+v east=%+v", west, east)
	}
}

func TestTeleportChurnRequiresQuarterablePopulation(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlayerFactory(loaded.Definition, ScenarioTeleportChurn, 10); err == nil {
		t.Fatal("expected teleport-churn population validation error")
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

	dx, dz = MovementDirection(ScenarioTeleportChurn, world.EntityID(1), 0)
	if dx != 0 || dz != 0 {
		t.Fatalf("teleport-churn direction = (%f,%f), want (0,0)", dx, dz)
	}
}
