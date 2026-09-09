package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/playerarchetype"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestJoinCanonicalizesDefaultPlayerArchetype(t *testing.T) {
	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	runtime := New(sim, DefaultConfig())
	connection := session.NewQueueConnection(8, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session:       s,
		Entity:        world.EntityState{ID: 10, Kind: world.EntityPlayer},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	entity, ok := sim.Entity(10)
	if !ok {
		t.Fatal("entity not spawned")
	}
	if entity.ArchetypeID != playerarchetype.DefaultID {
		t.Fatalf("authoritative archetype=%q want=%q", entity.ArchetypeID, playerarchetype.DefaultID)
	}
}

func TestCanonicalizeJoinEntityPreservesExplicitIdentity(t *testing.T) {
	player := canonicalizeJoinEntity(world.EntityState{ID: 1, Kind: world.EntityPlayer, ArchetypeID: "player_variant_test"})
	if player.ArchetypeID != "player_variant_test" {
		t.Fatalf("explicit player archetype=%q", player.ArchetypeID)
	}

	monster := canonicalizeJoinEntity(world.EntityState{ID: 2, Kind: world.EntityMonster})
	if monster.ArchetypeID != "" {
		t.Fatalf("monster archetype unexpectedly defaulted to %q", monster.ArchetypeID)
	}
}
