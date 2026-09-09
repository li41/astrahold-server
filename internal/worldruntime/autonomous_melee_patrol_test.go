package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeAgentConfigValidatesIdlePatrol(t *testing.T) {
	base := AutonomousMeleeAgentConfig{
		EntityID: 9001,
		Home: world.Position{Layer: 0},
		ActionID: "wolf-bite",
		AggroRange: 6,
		LeashRange: 8,
		AttackRange: 1.5,
		ReturnTolerance: 0.1,
	}

	cases := []struct {
		name   string
		mutate func(*AutonomousMeleeAgentConfig)
	}{
		{name: "patrol needs tolerance", mutate: func(c *AutonomousMeleeAgentConfig) {
			c.IdlePatrol = []world.Position{{X: 1, Layer: 0}}
		}},
		{name: "tolerance without patrol", mutate: func(c *AutonomousMeleeAgentConfig) {
			c.PatrolTolerance = 0.2
		}},
		{name: "wrong layer", mutate: func(c *AutonomousMeleeAgentConfig) {
			c.IdlePatrol = []world.Position{{X: 1, Layer: 1}}
			c.PatrolTolerance = 0.2
		}},
		{name: "outside leash", mutate: func(c *AutonomousMeleeAgentConfig) {
			c.IdlePatrol = []world.Position{{X: 9, Layer: 0}}
			c.PatrolTolerance = 0.2
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := base
			tc.mutate(&config)
			if err := validateAutonomousMeleeAgentConfig(config); !errors.Is(err, ErrInvalidAutonomousMeleeAgent) {
				t.Fatalf("err=%v want=%v", err, ErrInvalidAutonomousMeleeAgent)
			}
		})
	}

	valid := base
	valid.IdlePatrol = []world.Position{{X: 1, Layer: 0}, {X: 1, Z: 1, Layer: 0}}
	valid.PatrolTolerance = 0.2
	if err := validateAutonomousMeleeAgentConfig(valid); err != nil {
		t.Fatalf("valid patrol rejected: %v", err)
	}
}

func TestAutonomousMeleeAgentPatrolsWhileHealthyAndIdle(t *testing.T) {
	rt := newAutonomousPatrolTestRuntime(t, AutonomousMeleeAgentConfig{
		EntityID: 9001,
		Home: world.Position{Layer: 0},
		ActionID: "wolf-bite",
		AggroRange: 6,
		LeashRange: 8,
		AttackRange: 1.5,
		ReturnTolerance: 0.05,
		IdlePatrol: []world.Position{
			{X: 1, Layer: 0},
			{X: 1, Z: 1, Layer: 0},
			{Z: 1, Layer: 0},
			{Layer: 0},
		},
		PatrolTolerance: 0.05,
	})
	spawnAutonomousPatrolMonster(t, rt)

	assertCleanAutonomousPatrolStep(t, rt.Step(1, 50*time.Millisecond))
	first, ok := rt.world.Entity(9001)
	if !ok || first.Transform.Position.X <= 0 || first.Transform.Position.Z != 0 {
		t.Fatalf("first patrol step=%#v ok=%v; want movement toward first authored point", first, ok)
	}

	for tick := uint64(2); tick <= 8; tick++ {
		assertCleanAutonomousPatrolStep(t, rt.Step(tick, 50*time.Millisecond))
	}
	later, ok := rt.world.Entity(9001)
	if !ok {
		t.Fatal("patrolling monster disappeared")
	}
	agent := rt.autonomousMeleeAgents[0]
	if agent.targetID != 0 || agent.returningHome {
		t.Fatalf("idle patrol entered combat/evade state: %#v", agent)
	}
	if agent.patrolIndex != 1 {
		t.Fatalf("patrol index=%d want=1 after reaching first point", agent.patrolIndex)
	}
	if later.Transform.Position.Z <= 0 {
		t.Fatalf("later patrol position=%#v; want movement toward second authored point", later.Transform.Position)
	}
}

func TestAutonomousMeleeAgentStopsPatrolWhenWoundedAndEvadesHome(t *testing.T) {
	rt := newAutonomousPatrolTestRuntime(t, AutonomousMeleeAgentConfig{
		EntityID: 9001,
		Home: world.Position{Layer: 0},
		ActionID: "wolf-bite",
		AggroRange: 6,
		LeashRange: 8,
		AttackRange: 1.5,
		ReturnTolerance: 0.05,
		IdlePatrol: []world.Position{
			{X: 2, Layer: 0},
			{X: 2, Z: 1, Layer: 0},
			{Layer: 0},
		},
		PatrolTolerance: 0.05,
	})
	spawnAutonomousPatrolMonster(t, rt)

	for tick := uint64(1); tick <= 3; tick++ {
		assertCleanAutonomousPatrolStep(t, rt.Step(tick, 50*time.Millisecond))
	}
	before, ok := rt.world.Entity(9001)
	if !ok || before.Transform.Position.X <= 0 {
		t.Fatalf("monster did not begin patrol: %#v ok=%v", before, ok)
	}
	if _, err := rt.characters.ApplyDamage(9001, 50); err != nil {
		t.Fatal(err)
	}

	assertCleanAutonomousPatrolStep(t, rt.Step(4, 50*time.Millisecond))
	after, ok := rt.world.Entity(9001)
	if !ok || after.Transform.Position.X >= before.Transform.Position.X {
		t.Fatalf("wounded monster did not turn home: before=%#v after=%#v ok=%v", before.Transform.Position, after.Transform.Position, ok)
	}
	agent := rt.autonomousMeleeAgents[0]
	if !agent.returningHome || agent.patrolIndex != 0 || agent.targetID != 0 {
		t.Fatalf("wounded patrol state=%#v; want clean evade home", agent)
	}

	for tick := uint64(5); tick <= 20 && rt.autonomousMeleeAgents[0].returningHome; tick++ {
		assertCleanAutonomousPatrolStep(t, rt.Step(tick, 50*time.Millisecond))
	}
	if rt.autonomousMeleeAgents[0].returningHome {
		t.Fatal("wounded monster never completed return home")
	}
	state, ok := rt.characters.State(9001)
	if !ok || state.Defeated || state.HP != state.MaxHP || state.MP != state.MaxMP {
		t.Fatalf("returned vitals=%#v ok=%v; want full authoritative reset", state, ok)
	}
}

func newAutonomousPatrolTestRuntime(t *testing.T, agent AutonomousMeleeAgentConfig) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite",
		Targets: []combat.TargetKind{combat.TargetEntity},
		Range: 2,
		BaseDamage: 90,
		DamageType: combat.DamagePhysical,
		CooldownSeconds: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	return New(sim, cfg, WithCombatService(combatService), WithAutonomousMeleeAgent(agent))
}

func spawnAutonomousPatrolMonster(t *testing.T, rt *Runtime) {
	t.Helper()
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001,
			Kind: world.EntityMonster,
			ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{Layer: 0}},
		},
		Speed: 4,
		Radius: 0.35,
		MaxStepHeight: 0.5,
		HP: 200,
		MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertCleanAutonomousPatrolStep(t *testing.T, report StepReport) {
	t.Helper()
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 || len(report.TickErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("step report=%#v", report)
	}
}
