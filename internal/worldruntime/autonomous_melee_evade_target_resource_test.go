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
	"github.com/li41/astrahold-server/internal/targetresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeEvadeClearsTargetResource(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
		BaseDamage: 5, DamageType: combat.DamagePhysical, CooldownSeconds: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}

	const (
		sourceID  world.EntityID = 10
		monsterID world.EntityID = 9001
	)
	home := world.Position{Layer: 0}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(
		sim,
		cfg,
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: monsterID, Home: home, ActionID: "wolf-bite",
			AggroRange: 4, LeashRange: 10, AttackRange: 1,
			ReturnTolerance: 0.05,
		}),
	)

	conn := session.NewQueueConnection(128, 16)
	source, err := session.New(1, sourceID, 32, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: source,
		Entity: world.EntityState{
			ID: sourceID, Kind: world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{X: 50, Layer: 0}},
		},
		Speed: 6, Radius: .35, MaxStepHeight: .5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: monsterID, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: home},
		},
		Speed: 4, Radius: .35, MaxStepHeight: .5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("initial report=%#v", report)
	}
	drainReliable(conn)

	state, changed, err := rt.characters.GainTargetResource(sourceID, monsterID, targetresource.Flaw, 1, 3, 1, 51)
	if err != nil || !changed || state.Current != 1 {
		t.Fatalf("seed flaw state=%+v changed=%v err=%v", state, changed, err)
	}

	if err := rt.EnqueueTeleport(monsterID, world.Position{X: 12, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("evade report=%#v", report)
	}
	if _, ok := rt.characters.TargetResourceState(sourceID, monsterID, targetresource.Flaw); ok {
		t.Fatal("target resource survived authoritative monster evade")
	}
	if len(rt.autonomousMeleeAgents) != 1 || !rt.autonomousMeleeAgents[0].returningHome {
		t.Fatalf("agent state=%#v, want returning home", rt.autonomousMeleeAgents)
	}
	assertTargetResourceClearMessage(t, drainReliable(conn), sourceID, monsterID)
}
