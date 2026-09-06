package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeAgentChasesAndDamagesPlayerThroughCombatAuthority(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}},
	}, 0, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
		BaseDamage: 90, DamageType: combat.DamagePhysical, CooldownSeconds: 1.35,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	cfg.CharacterMaxHP = 200
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001, Home: world.Position{X: 5, Layer: 0}, ActionID: "wolf-bite",
			AggroRange: 10, LeashRange: 15, AttackRange: 1.75, ReturnTolerance: 0.1,
		}),
	)

	conn := session.NewQueueConnection(256, 256)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{X: 5, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}

	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 || len(initial.ActionRejections) != 0 {
		t.Fatalf("initial=%#v", initial)
	}
	drainConnection(conn)

	hit := false
	for tick := uint64(2); tick <= 30; tick++ {
		report := rt.Step(tick, 50*time.Millisecond)
		if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
			t.Fatalf("tick=%d report=%#v", tick, report)
		}
		if report.Metrics.EntityActionsApplied > 0 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("wolf never reached and damaged the player")
	}

	playerState, ok := rt.characters.State(1)
	if !ok || playerState.HP != 110 || playerState.Defeated {
		t.Fatalf("player state=%#v ok=%v; want hp=110 alive", playerState, ok)
	}
	wolf, ok := rt.world.Entity(9001)
	if !ok || wolf.Transform.Position.X >= 5 {
		t.Fatalf("wolf=%#v ok=%v; want authoritative chase toward player", wolf, ok)
	}

	sawStarted := false
	sawHit := false
	for {
		select {
		case env := <-conn.Reliable():
			switch message := env.Message.(type) {
			case protocol.ActionStarted:
				if message.ActorEntityID == 9001 && message.ActionID == "wolf-bite" && message.TargetID == "1" {
					sawStarted = true
				}
			case protocol.CombatEvent:
				if message.ActorEntityID == 9001 && message.TargetEntityID == 1 && message.ActionID == "wolf-bite" && message.Damage == 90 {
					sawHit = true
				}
			}
		default:
			if !sawStarted || !sawHit {
				t.Fatalf("missing authoritative combat presentation events: started=%v hit=%v", sawStarted, sawHit)
			}
			return
		}
	}
}

func TestAutonomousMeleeAgentDropsTargetOutsideLeashAndReturnsHome(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 4, Layer: 0}},
	}, 0, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
		BaseDamage: 90, DamageType: combat.DamagePhysical, CooldownSeconds: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001, Home: world.Position{X: 0, Layer: 0}, ActionID: "wolf-bite",
			AggroRange: 6, LeashRange: 8, AttackRange: 1.5, ReturnTolerance: 0.05,
		}),
	)
	conn := session.NewQueueConnection(32, 32)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}

	for tick := uint64(1); tick <= 2; tick++ {
		report := rt.Step(tick, 50*time.Millisecond)
		if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
			t.Fatalf("tick=%d report=%#v", tick, report)
		}
	}
	before, ok := rt.world.Entity(9001)
	if !ok || before.Transform.Position.X <= 0 {
		t.Fatalf("wolf before leash=%#v ok=%v; want it chasing away from home", before, ok)
	}

	if err := rt.EnqueueTeleport(1, world.Position{X: 20, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("leash report=%#v", report)
	}
	after, ok := rt.world.Entity(9001)
	if !ok || after.Transform.Position.X >= before.Transform.Position.X {
		t.Fatalf("wolf after leash=%#v before=%#v; want return toward home", after, before)
	}
	if got := rt.autonomousMeleeAgents[0].targetID; got != 0 {
		t.Fatalf("target=%d; want cleared after player leaves leash", got)
	}
}
