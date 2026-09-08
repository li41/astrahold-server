package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeAgentTracksTargetFacingWhileStationary(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1,
		Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 1, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite",
		Targets: []combat.TargetKind{combat.TargetEntity},
		Range: 2,
		BaseDamage: 10,
		DamageType: combat.DamagePhysical,
		CooldownSeconds: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	cfg.CharacterMaxHP = 200
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001,
			Home: world.Position{Layer: 0},
			ActionID: "wolf-bite",
			AggroRange: 6,
			LeashRange: 8,
			AttackRange: 1.5,
			ReturnTolerance: 0.05,
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
			ID: 9001,
			Kind: world.EntityMonster,
			ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{Layer: 0}, Yaw: 180},
		},
		Speed: 4,
		Radius: 0.35,
		MaxStepHeight: 0.5,
		HP: 200,
		MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}

	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 {
		t.Fatalf("tick 1 report=%#v", report)
	}
	wolf, ok := rt.world.Entity(9001)
	if !ok {
		t.Fatal("wolf disappeared")
	}
	if wolf.Transform.Position.X != 0 || wolf.Transform.Position.Z != 0 {
		t.Fatalf("wolf moved while already in melee range: %+v", wolf.Transform.Position)
	}
	if wolf.Transform.Yaw < -0.01 || wolf.Transform.Yaw > 0.01 {
		t.Fatalf("wolf yaw=%f want=0 while player is east", wolf.Transform.Yaw)
	}

	if err := rt.EnqueueTeleport(1, world.Position{Z: 1, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 {
		t.Fatalf("tick 2 report=%#v", report)
	}
	wolf, ok = rt.world.Entity(9001)
	if !ok {
		t.Fatal("wolf disappeared after player circled")
	}
	if wolf.Transform.Position.X != 0 || wolf.Transform.Position.Z != 0 {
		t.Fatalf("stationary facing introduced movement: %+v", wolf.Transform.Position)
	}
	if wolf.Transform.Yaw < 89.99 || wolf.Transform.Yaw > 90.01 {
		t.Fatalf("wolf yaw=%f want=90 after player circles north", wolf.Transform.Yaw)
	}
}
