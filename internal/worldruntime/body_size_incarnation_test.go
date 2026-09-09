package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCurrentEntityBodySizeDoesNotInheritLifecycleConfigForSameID(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	entity := world.EntityState{ID: 9001, Kind: world.EntityMonster, ArchetypeID: "new-unclassified-incarnation"}
	if err := sim.Spawn(entity, 4, .35, .5); err != nil {
		t.Fatal(err)
	}

	// The lifecycle config deliberately carries a stale Large classification for the same ID.
	// The current EntityState is a different, unclassified incarnation and must remain the truth.
	lifecycle := MonsterLifecycleConfig{
		Spawn: SpawnEntityRequest{
			Entity: world.EntityState{ID: 9001, Kind: world.EntityMonster, ArchetypeID: "old-large-incarnation"},
			Speed: 4, Radius: .35, MaxStepHeight: .5,
			HP: 100, MaxHP: 100, BodySize: equipmentcatalog.BodySizeLarge,
		},
		CorpseHoldTicks: 2,
		RespawnDelayTicks: 8,
	}
	rt := New(sim, DefaultConfig(), WithMonsterLifecycle(lifecycle))

	current, ok := rt.world.Entity(9001)
	if !ok || current.BodySize != "" {
		t.Fatalf("current entity=%#v ok=%v", current, ok)
	}
	if got := rt.entityWeaponBodySize(9001); got != equipmentcatalog.BodySizeSmall {
		t.Fatalf("current unclassified incarnation inherited stale lifecycle size: got=%q want=%q", got, equipmentcatalog.BodySizeSmall)
	}
}
