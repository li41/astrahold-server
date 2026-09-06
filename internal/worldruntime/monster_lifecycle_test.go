package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMonsterLifecycleCorpseDespawnAndRespawn(t *testing.T) {
	spawn := testMonsterLifecycleSpawn()
	rt := newMonsterLifecycleRuntime(t, spawn, 2, 2)

	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLifecycleStep(t, rt.Step(1, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, false, spawn.MaxHP)

	defeated, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP)
	if err != nil {
		t.Fatal(err)
	}
	if !defeated.Defeated || defeated.HP != 0 {
		t.Fatalf("defeated state=%+v", defeated)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)

	assertCleanMonsterLifecycleStep(t, rt.Step(2, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, true, 0)
	if got := rt.monsterLifecycles[0].phase; got != monsterLifecycleCorpse {
		t.Fatalf("phase=%d want corpse", got)
	}

	assertCleanMonsterLifecycleStep(t, rt.Step(3, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, true, 0)

	assertCleanMonsterLifecycleStep(t, rt.Step(4, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, false, false, 0)
	if got := rt.monsterLifecycles[0].phase; got != monsterLifecycleWaitingRespawn {
		t.Fatalf("phase=%d want waiting respawn", got)
	}

	assertCleanMonsterLifecycleStep(t, rt.Step(5, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, false, false, 0)

	assertCleanMonsterLifecycleStep(t, rt.Step(6, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, false, spawn.MaxHP)
	if got := rt.monsterLifecycles[0].phase; got != monsterLifecycleActive {
		t.Fatalf("phase=%d want active", got)
	}
}

func TestMonsterLifecycleWaitsForReliableDespawnBeforeEntityIDReuse(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}},
	}, 4, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	spawn := testMonsterLifecycleSpawn()
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	rt := New(sim, cfg, WithMonsterLifecycle(MonsterLifecycleConfig{
		Spawn: spawn, CorpseHoldTicks: 1, RespawnDelayTicks: 1,
	}))
	conn := &monsterLifecycleTestConnection{}
	s, err := session.New(1, 1, 50, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLifecycleStep(t, rt.Step(1, 50*time.Millisecond))
	if !rt.replication.KnownByAny(spawn.Entity.ID) {
		t.Fatal("monster should be known after initial reliable spawn")
	}

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLifecycleStep(t, rt.Step(2, 50*time.Millisecond))
	if !conn.sawDefeatedVitals(spawn.Entity.ID) {
		t.Fatal("missing authoritative defeated vitals before corpse removal")
	}

	conn.blockDespawn = true
	assertCleanMonsterLifecycleStep(t, rt.Step(3, 50*time.Millisecond))
	if _, exists := rt.world.Entity(spawn.Entity.ID); exists {
		t.Fatal("corpse should be removed from world after defeated vitals converge")
	}
	if !rt.replication.KnownByAny(spawn.Entity.ID) {
		t.Fatal("failed despawn delivery must preserve old incarnation knowledge")
	}

	assertCleanMonsterLifecycleStep(t, rt.Step(4, 50*time.Millisecond))
	if _, exists := rt.world.Entity(spawn.Entity.ID); exists {
		t.Fatal("monster must not reuse EntityID while old despawn is backpressured")
	}

	conn.blockDespawn = false
	assertCleanMonsterLifecycleStep(t, rt.Step(5, 50*time.Millisecond))
	if rt.replication.KnownByAny(spawn.Entity.ID) {
		t.Fatal("despawn acceptance should clear old incarnation knowledge")
	}
	if _, exists := rt.world.Entity(spawn.Entity.ID); exists {
		t.Fatal("respawn should wait until the world-owner observes cleared knowledge on next step")
	}

	assertCleanMonsterLifecycleStep(t, rt.Step(6, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, false, spawn.MaxHP)
	if conn.spawnCount(spawn.Entity.ID) < 2 {
		t.Fatalf("spawn count=%d want initial + fresh respawn", conn.spawnCount(spawn.Entity.ID))
	}
}

func TestMonsterLifecycleRejectsDuplicateEntityID(t *testing.T) {
	spawn := testMonsterLifecycleSpawn()
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(8), movement.NewService(nav, 0.1))
	cfg := MonsterLifecycleConfig{Spawn: spawn, CorpseHoldTicks: 1, RespawnDelayTicks: 1}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate managed monster EntityID should panic during runtime construction")
		}
	}()
	_ = New(sim, DefaultConfig(), WithMonsterLifecycle(cfg), WithMonsterLifecycle(cfg))
}

func TestLifecycleTickAfterSaturates(t *testing.T) {
	if got := lifecycleTickAfter(^uint64(0)-1, 5); got != ^uint64(0) {
		t.Fatalf("saturated tick=%d want max uint64", got)
	}
}

type monsterLifecycleTestConnection struct {
	blockDespawn bool
	messages     []protocol.Envelope
}

func (c *monsterLifecycleTestConnection) TrySend(envelope protocol.Envelope) error {
	if _, ok := envelope.Message.(protocol.EntityDespawn); ok && c.blockDespawn {
		return session.ErrBackpressure
	}
	c.messages = append(c.messages, envelope)
	return nil
}

func (c *monsterLifecycleTestConnection) Close() error { return nil }

func (c *monsterLifecycleTestConnection) sawDefeatedVitals(entityID world.EntityID) bool {
	for _, envelope := range c.messages {
		if vitals, ok := envelope.Message.(protocol.EntityVitalsState); ok && vitals.EntityID == entityID && vitals.HP == 0 && vitals.Defeated {
			return true
		}
	}
	return false
}

func (c *monsterLifecycleTestConnection) spawnCount(entityID world.EntityID) int {
	count := 0
	for _, envelope := range c.messages {
		if spawn, ok := envelope.Message.(protocol.EntitySpawn); ok && spawn.EntityID == entityID {
			count++
		}
	}
	return count
}

func testMonsterLifecycleSpawn() SpawnEntityRequest {
	return SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}
}

func newMonsterLifecycleRuntime(t *testing.T, spawn SpawnEntityRequest, corpseHold, respawnDelay uint64) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	return New(sim, cfg, WithMonsterLifecycle(MonsterLifecycleConfig{
		Spawn: spawn, CorpseHoldTicks: corpseHold, RespawnDelayTicks: respawnDelay,
	}))
}

func assertCleanMonsterLifecycleStep(t *testing.T, report StepReport) {
	t.Helper()
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("step report=%#v", report)
	}
}

func assertMonsterLifecycleState(t *testing.T, rt *Runtime, entityID world.EntityID, wantWorld, wantDefeated bool, wantHP uint32) {
	t.Helper()
	_, inWorld := rt.world.Entity(entityID)
	if inWorld != wantWorld {
		t.Fatalf("world membership=%v want=%v", inWorld, wantWorld)
	}
	state, hasState := rt.characters.State(entityID)
	if !wantWorld {
		if hasState {
			t.Fatalf("character state should be removed with world entity: %+v", state)
		}
		return
	}
	if !hasState || state.Defeated != wantDefeated || state.HP != wantHP {
		t.Fatalf("character state=%+v exists=%v want defeated=%v hp=%d", state, hasState, wantDefeated, wantHP)
	}
}
