package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAuthoritativeGateBreachAdvancesSiegeMatchToThrone(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "siege-match-test", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10},
			Plane: gameplayworld.SurfacePlane{},
		}},
		Blockers: []gameplayworld.Blocker{{
			ID: "main-gate", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2},
			MinY: 0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true,
		}},
		Gates: []gameplayworld.Gate{{
			ID: "main-gate", BlockerID: "main-gate", MaxHP: 100,
			Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 100, CooldownSeconds: 0.5},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(nav),
		WithSiegeGates(definition.Gates),
		WithSiegeMatch(siege.MatchDefinition{
			ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne",
		}),
	)
	conn := session.NewQueueConnection(16, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	initialReport := rt.Step(1, 50*time.Millisecond)
	if len(initialReport.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", initialReport.CommandErrors)
	}
	initial, ok := rt.SiegeMatchState()
	if !ok || initial.Phase != siege.MatchPhaseGate || initial.GateBreached || initial.Revision != 1 {
		t.Fatalf("initial match=%+v ok=%v", initial, ok)
	}

	if err := rt.EnqueueAttackGate(1, 1, "main-gate"); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("breach report=%#v", report)
	}
	advanced, ok := rt.SiegeMatchState()
	if !ok || advanced.Phase != siege.MatchPhaseThrone || !advanced.GateBreached || advanced.Revision != 2 {
		t.Fatalf("advanced match=%+v ok=%v", advanced, ok)
	}
	if enabled, err := nav.BlockerEnabled("main-gate"); err != nil || enabled {
		t.Fatalf("authoritative blocker enabled=%v err=%v", enabled, err)
	}
}
