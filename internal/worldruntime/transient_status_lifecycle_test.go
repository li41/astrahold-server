package worldruntime

import (
	"strconv"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestPlayerDefeatClearsAllTransientStatuses(t *testing.T) {
	rt, s, _, monsterID := newOathguardFortifyRuntime(t)
	armFortifyForLifecycleTest(t, rt, s.ID, s.EntityID)

	if _, err := rt.characters.ApplyDamage(s.EntityID, 950); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(s.EntityID)
	applyFortifyTestWolfBite(t, rt, monsterID, 3)

	state, ok := rt.characters.State(s.EntityID)
	if !ok || !state.Defeated || state.HP != 0 {
		t.Fatalf("player state=%+v exists=%v want defeated hp=0", state, ok)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 3); got != 0 {
		t.Fatalf("transient reduction survived player defeat: %d", got)
	}
}

func TestPlayerLeaveClearsAllTransientStatuses(t *testing.T) {
	rt, s, _, _ := newOathguardFortifyRuntime(t)
	armFortifyForLifecycleTest(t, rt, s.ID, s.EntityID)

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

func armFortifyForLifecycleTest(t *testing.T, rt *Runtime, sessionID session.ID, entityID world.EntityID) {
	t.Helper()
	if _, err := rt.characters.AssignInitialClass(entityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(entityID, classresource.Resolve, 30); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(sessionID, 1, protocol.ClientUseAction{
		ActionID:   classaction.OathguardFortify,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   strconv.FormatUint(uint64(entityID), 10),
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("fortify report=%#v", report)
	}
	if got := rt.combat.SelfDamageReductionPercent(entityID, 2); got != 45 {
		t.Fatalf("active reduction=%d want=45", got)
	}
}
