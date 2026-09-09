package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeThreatSwitchesOnActualDamageAndFallsBackAfterDisconnect(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "threat-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	for _, player := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 3, Layer: 0}}},
	} {
		if err := sim.Spawn(player, 0, .35, .5); err != nil {
			t.Fatal(err)
		}
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{
		{ID: "jab", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 10, DamageType: combat.DamagePhysical, CooldownSeconds: .5},
		{ID: "heavy-jab", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 20, DamageType: combat.DamagePhysical, CooldownSeconds: .5},
		{ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 1, BaseDamage: 5, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(nav),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001, Home: world.Position{Layer: 0}, ActionID: "wolf-bite",
			AggroRange: 6, LeashRange: 8, AttackRange: .25, ReturnTolerance: .05,
		}),
	)

	conn1 := session.NewQueueConnection(128, 16)
	player1, err := session.New(1, 1, 32, conn1)
	if err != nil {
		t.Fatal(err)
	}
	conn2 := session.NewQueueConnection(128, 16)
	player2, err := session.New(2, 2, 32, conn2)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(player1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(player2); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: "wolf-threat-test",
			Transform: world.Transform{Position: world.Position{Layer: 0}},
		},
		Speed: 4, Radius: .35, MaxStepHeight: .5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("initial report=%#v", report)
	}
	agent := &rt.autonomousMeleeAgents[0]
	if agent.targetID != 1 {
		t.Fatalf("initial proximity target=%d, want nearest player 1", agent.targetID)
	}

	if err := rt.EnqueueUseAction(player1.ID, 1, protocol.ClientUseAction{ActionID: "jab", TargetKind: protocol.ActionTargetEntity, TargetID: "9001"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(player2.ID, 1, protocol.ClientUseAction{ActionID: "heavy-jab", TargetKind: protocol.ActionTargetEntity, TargetID: "9001"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("damage report=%#v", report)
	}
	if got := agent.threat.Value(1); got != 10 {
		t.Fatalf("player 1 threat=%d, want actual damage 10", got)
	}
	if got := agent.threat.Value(2); got != 20 {
		t.Fatalf("player 2 threat=%d, want actual damage 20", got)
	}
	if agent.targetID != 2 {
		t.Fatalf("threat target=%d, want higher-threat player 2", agent.targetID)
	}
	monsterState, ok := rt.combatantState(9001)
	if !ok || monsterState.HP != 170 {
		t.Fatalf("monster state=%#v ok=%v, want hp=170", monsterState, ok)
	}

	// Unregister deliberately leaves the world entity alive. The AI must use active Server session
	// ownership when pruning threat, otherwise a disconnected ghost could remain the combat target.
	if err := rt.EnqueueUnregister(player2.ID); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("player2 disconnect report=%#v", report)
	}
	if agent.targetID != 1 {
		t.Fatalf("fallback target=%d, want remaining threatened player 1", agent.targetID)
	}
	if got := agent.threat.Value(2); got != 0 {
		t.Fatalf("disconnected player threat not pruned: %d", got)
	}

	if err := rt.EnqueueUnregister(player1.ID); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("player1 disconnect report=%#v", report)
	}
	if agent.threat.Len() != 0 {
		t.Fatalf("threat len after all targets invalid=%d, want 0", agent.threat.Len())
	}
	if agent.targetID != 0 || !agent.returningHome {
		t.Fatalf("agent after all targets invalid: target=%d returning=%v", agent.targetID, agent.returningHome)
	}
}
