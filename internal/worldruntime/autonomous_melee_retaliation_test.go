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

func TestAutonomousMeleeMonsterRetaliatesAfterRangedHitOutsideProximityAggro(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1,
		Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: "fireball",
			Targets: []combat.TargetKind{combat.TargetPoint},
			Range: 12,
			HitRadius: 0.9,
			PointResolution: combat.PointResolutionEndpointNearest,
			BaseDamage: 50,
			DamageType: combat.DamagePhysical,
			MPCost: 10,
			CooldownSeconds: 1,
		},
		{
			ID: "wolf-bite",
			Targets: []combat.TargetKind{combat.TargetEntity},
			Range: 2,
			BaseDamage: 10,
			DamageType: combat.DamagePhysical,
			CooldownSeconds: 1,
		},
	})
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
			Home: world.Position{X: 11, Layer: 0},
			ActionID: "wolf-bite",
			AggroRange: 9,
			LeashRange: 16,
			AttackRange: 1.75,
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
			Transform: world.Transform{Position: world.Position{X: 11, Layer: 0}},
		},
		Speed: 4,
		Radius: 0.35,
		MaxStepHeight: 0.5,
		HP: 200,
		MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}
	targetX := float32(11)
	targetZ := float32(0)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{
		ActionID: "fireball",
		TargetKind: protocol.ActionTargetPoint,
		TargetX: &targetX,
		TargetZ: &targetZ,
	}); err != nil {
		t.Fatal(err)
	}

	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 {
		t.Fatalf("ranged opening report=%#v", report)
	}
	state, ok := rt.characters.State(9001)
	if !ok || state.Defeated || state.HP != 150 {
		t.Fatalf("wolf vitals=%#v ok=%v; want hp=150 alive, not evade-healed", state, ok)
	}
	agent := rt.autonomousMeleeAgents[0]
	if agent.targetID != 1 || agent.returningHome {
		t.Fatalf("retaliation state=%#v; want player 1 target without evade", agent)
	}
	wolf, ok := rt.world.Entity(9001)
	if !ok || wolf.Transform.Position.X >= 11 {
		t.Fatalf("wolf=%#v ok=%v; want authoritative chase toward ranged attacker", wolf, ok)
	}
}

func TestProvokeAutonomousMeleeMonsterDoesNotReplaceActiveTargetOrInterruptEvade(t *testing.T) {
	rt := &Runtime{autonomousMeleeAgents: []autonomousMeleeAgent{{
		config: AutonomousMeleeAgentConfig{EntityID: 9001},
		targetID: 1,
	}}}

	rt.provokeAutonomousMeleeMonster(9001, 2)
	if got := rt.autonomousMeleeAgents[0].targetID; got != 1 {
		t.Fatalf("active target switched to %d; want existing target 1", got)
	}

	rt.autonomousMeleeAgents[0].targetID = 0
	rt.autonomousMeleeAgents[0].returningHome = true
	rt.provokeAutonomousMeleeMonster(9001, 2)
	if got := rt.autonomousMeleeAgents[0].targetID; got != 0 {
		t.Fatalf("evading monster acquired retaliation target %d", got)
	}
}
