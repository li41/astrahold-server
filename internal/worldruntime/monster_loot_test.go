package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	testLootSourceArchetypeID = "monster-loot-test"
	testLootItemArchetypeID   = "item_loot_test"
)

func TestMonsterLootDropsOnceOnAuthoritativeDefeatWithoutClientAction(t *testing.T) {
	rt, sim := newMonsterLootTestRuntime(t, testMonsterLootCatalog(t))
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))

	drops := itemDrops(sim)
	if len(drops) != 1 || drops[0].ArchetypeID != testLootItemArchetypeID {
		t.Fatalf("drops=%#v", drops)
	}
	firstID := drops[0].ID
	assertCleanMonsterLootStep(t, rt.Step(3, 50*time.Millisecond))
	drops = itemDrops(sim)
	if len(drops) != 1 || drops[0].ID != firstID {
		t.Fatalf("defeated monster duplicated loot: %#v", drops)
	}
}

func TestMonsterLootIgnoresUnconfiguredArchetype(t *testing.T) {
	rt, sim := newMonsterLootTestRuntime(t, testMonsterLootCatalog(t))
	spawn := testMonsterLootSpawn("monster-unconfigured")
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	if drops := itemDrops(sim); len(drops) != 0 {
		t.Fatalf("unconfigured monster produced drops: %#v", drops)
	}
}

func TestMonsterLootRearmsAfterLifecycleRespawn(t *testing.T) {
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	rt := New(
		sim,
		cfg,
		WithMonsterLootCatalog(testMonsterLootCatalog(t)),
		WithMonsterLifecycle(MonsterLifecycleConfig{Spawn: spawn, CorpseHoldTicks: 1, RespawnDelayTicks: 1}),
	)
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	first := itemDrops(sim)
	if len(first) != 1 {
		t.Fatalf("first drops=%#v", first)
	}
	firstID := first[0].ID

	assertCleanMonsterLootStep(t, rt.Step(3, 50*time.Millisecond))
	assertCleanMonsterLootStep(t, rt.Step(4, 50*time.Millisecond))
	if state, ok := rt.characters.State(spawn.Entity.ID); !ok || state.Defeated || state.HP != spawn.MaxHP {
		t.Fatalf("respawned state=%+v ok=%v", state, ok)
	}

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(5, 50*time.Millisecond))
	drops := itemDrops(sim)
	if len(drops) != 2 {
		t.Fatalf("second incarnation drops=%#v", drops)
	}
	if drops[0].ID == drops[1].ID || (drops[0].ID != firstID && drops[1].ID != firstID) {
		t.Fatalf("drop identities not fresh: first=%d drops=%#v", firstID, drops)
	}
}

func TestMonsterDefeatLootPickupUpdatesInventory(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithMonsterLootCatalog(testMonsterLootCatalog(t)))
	connection := session.NewQueueConnection(64, 16)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	drops := itemDrops(sim)
	if len(drops) != 1 {
		t.Fatalf("drops=%#v", drops)
	}
	if err := rt.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: drops[0].ID}); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(3, 50*time.Millisecond))
	if _, exists := sim.Entity(drops[0].ID); exists {
		t.Fatal("picked loot still exists in authoritative world")
	}
	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil || inv.Quantity(testLootItemArchetypeID) != 1 {
		t.Fatalf("inventory=%v quantity=%d", inv, func() uint32 {
			if inv == nil {
				return 0
			}
			return inv.Quantity(testLootItemArchetypeID)
		}())
	}
}

func newMonsterLootTestRuntime(t *testing.T, catalog *loot.Catalog) (*Runtime, *simulation.World) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	return New(sim, cfg, WithMonsterLootCatalog(catalog)), sim
}

func testMonsterLootCatalog(t *testing.T) *loot.Catalog {
	t.Helper()
	catalog, err := loot.New(loot.Definition{
		Revision: "monster-loot-test-v1",
		Tables: []loot.Table{{
			SourceArchetypeID: testLootSourceArchetypeID,
			Drops:             []loot.Drop{{ItemArchetypeID: testLootItemArchetypeID}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func testMonsterLootSpawn(archetypeID string) SpawnEntityRequest {
	return SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: archetypeID,
			Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}
}

func itemDrops(sim *simulation.World) []world.EntityState {
	var drops []world.EntityState
	for _, entity := range sim.Snapshot() {
		if entity.Kind == world.EntityItemDrop {
			drops = append(drops, entity)
		}
	}
	return drops
}

func assertCleanMonsterLootStep(t *testing.T, report StepReport) {
	t.Helper()
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("step report=%#v", report)
	}
}
