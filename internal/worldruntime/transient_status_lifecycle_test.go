package worldruntime

import (
	"strconv"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const testTransientMitigationAction = "test-self-mitigation"

func TestPlayerDefeatClearsAllTransientStatuses(t *testing.T) {
	rt, s, monsterID := newTransientStatusPlayerRuntime(t)
	armTransientMitigationForLifecycleTest(t, rt, s.EntityID)

	if _, err := rt.characters.ApplyDamage(s.EntityID, 950); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(s.EntityID)
	applyTransientTestWolfBite(t, rt, monsterID, 3)

	state, ok := rt.characters.State(s.EntityID)
	if !ok || !state.Defeated || state.HP != 0 {
		t.Fatalf("player state=%+v exists=%v want defeated hp=0", state, ok)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 3); got != 0 {
		t.Fatalf("transient reduction survived player defeat: %d", got)
	}
}

func TestPlayerLeaveClearsAllTransientStatuses(t *testing.T) {
	rt, s, _ := newTransientStatusPlayerRuntime(t)
	armTransientMitigationForLifecycleTest(t, rt, s.EntityID)

	if err := rt.EnqueueLeave(s.ID); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 3); got != 0 {
		t.Fatalf("transient reduction survived player leave: %d", got)
	}
	if _, exists := rt.world.Entity(s.EntityID); exists {
		t.Fatal("player entity survived formal leave")
	}
}

func TestMonsterDefeatClearsTransientStatusesBeforeRespawn(t *testing.T) {
	spawn := testMonsterLifecycleSpawn()
	rt := newMonsterLifecycleRuntime(t, spawn, 2, 2)
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:                         "test-monster-transient",
		Effect:                     combat.EffectSelfMitigation,
		Targets:                    []combat.TargetKind{combat.TargetEntity},
		Range:                      0.1,
		SelfDamageReductionPercent: 45,
		DurationSeconds:            3,
		CooldownSeconds:            20,
	}})
	if err != nil {
		t.Fatal(err)
	}
	rt.combat = combatService

	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLifecycleStep(t, rt.Step(1, 50*time.Millisecond))

	prepared, err := rt.combat.Prepare(
		spawn.Entity.ID,
		"test-monster-transient",
		combat.Target{Kind: combat.TargetEntity, ID: strconv.FormatUint(uint64(spawn.Entity.ID), 10)},
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	rt.combat.Commit(prepared, 1, 50*time.Millisecond)
	if got := rt.combat.SelfDamageReductionPercent(spawn.Entity.ID, 1); got != 45 {
		t.Fatalf("active monster reduction=%d want=45", got)
	}

	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLifecycleStep(t, rt.Step(2, 50*time.Millisecond))
	if got := rt.combat.SelfDamageReductionPercent(spawn.Entity.ID, 2); got != 0 {
		t.Fatalf("transient reduction survived monster defeat: %d", got)
	}

	assertCleanMonsterLifecycleStep(t, rt.Step(3, 50*time.Millisecond))
	assertCleanMonsterLifecycleStep(t, rt.Step(4, 50*time.Millisecond))
	assertCleanMonsterLifecycleStep(t, rt.Step(5, 50*time.Millisecond))
	assertCleanMonsterLifecycleStep(t, rt.Step(6, 50*time.Millisecond))
	assertMonsterLifecycleState(t, rt, spawn.Entity.ID, true, false, spawn.MaxHP)
	if got := rt.combat.SelfDamageReductionPercent(spawn.Entity.ID, 6); got != 0 {
		t.Fatalf("old transient reduction leaked into respawned incarnation: %d", got)
	}
}

func newTransientStatusPlayerRuntime(t *testing.T) (*Runtime, *session.Session, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "transient-status-lifecycle-test",
		Revision:      "classless-r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	monsterID := world.EntityID(9104)
	if err := sim.Spawn(world.EntityState{
		ID: monsterID, Kind: world.EntityMonster,
		Transform: world.Transform{Position: world.Position{X: 1, Layer: 0}},
	}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: testTransientMitigationAction, Effect: combat.EffectSelfMitigation,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: .1,
			SelfDamageReductionPercent: 45, DurationSeconds: 3, CooldownSeconds: 20,
		},
		{
			ID: "wolf-bite", Effect: combat.EffectDamage,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
			BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: 1.35,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 500, MaxHP: 500, MP: 100, MaxMP: 100}); err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(128, 16)
	s, err := session.New(1, 10, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: .35, MaxStepHeight: .5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	return rt, s, monsterID
}

func armTransientMitigationForLifecycleTest(t *testing.T, rt *Runtime, entityID world.EntityID) {
	t.Helper()
	prepared, err := rt.combat.Prepare(
		entityID,
		testTransientMitigationAction,
		combat.Target{Kind: combat.TargetEntity, ID: strconv.FormatUint(uint64(entityID), 10)},
		2,
	)
	if err != nil {
		t.Fatal(err)
	}
	rt.combat.Commit(prepared, 2, 50*time.Millisecond)
	if got := rt.combat.SelfDamageReductionPercent(entityID, 2); got != 45 {
		t.Fatalf("active reduction=%d want=45", got)
	}
}

func applyTransientTestWolfBite(t *testing.T, rt *Runtime, monsterID world.EntityID, tick uint64) {
	t.Helper()
	prepared, err := rt.combat.Prepare(monsterID, "wolf-bite", combat.Target{Kind: combat.TargetEntity, ID: "10"}, tick)
	if err != nil {
		t.Fatal(err)
	}
	report := StepReport{Tick: tick}
	rt.dispatchPreparedAction("test-wolf-bite", 0, 0, prepared, tick, 50*time.Millisecond, &report)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("wolf bite report=%#v", report)
	}
}
