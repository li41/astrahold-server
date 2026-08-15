package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestQueuedNextSiegeRoundRestoresGateAndBumpsDynamicRevision(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "round-reset-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10},
			Plane:  gameplayworld.SurfacePlane{},
		}},
		Blockers: []gameplayworld.Blocker{{
			ID: "main-gate", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2},
			MinY:   0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true,
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
	throne := siege.ThroneObjectiveDefinition{
		ID:              "throne",
		Zone:            siege.ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8}},
		CaptureDuration: time.Millisecond,
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(nav),
		WithSiegeGates(definition.Gates),
		WithSiegeMatch(siege.MatchDefinition{
			ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne,
		}),
	)
	if err := rt.siege.AssignParticipant(1, siege.TeamAttacker); err != nil {
		t.Fatal(err)
	}
	gateState, err := rt.siege.Attack(1, world.Position{Z: 0, Layer: 0}, "main-gate", 1, 50*time.Millisecond, nav)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.siege.ObserveGateState(gateState) {
		t.Fatal("expected gate breach")
	}
	rt.siege.ObserveThronePresence([]siege.ParticipantPresence{{EntityID: 1, Position: world.Position{Z: 6, Layer: 0}}})
	rt.siege.AdvanceThroneCapture(time.Millisecond)
	if !rt.siege.ResolveThroneCapture() {
		t.Fatal("expected completed round")
	}
	rt.bumpDynamicRevision()
	beforeDynamic := rt.dynamicRevision

	if err := rt.EnqueueStartNextSiegeRound(); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("reset report=%#v", report)
	}
	match, _ := rt.SiegeMatchState()
	if match.Round != 2 || match.Phase != siege.MatchPhaseGate || match.AttackerID != "defenders" || match.DefenderID != "attackers" || match.GateBreached || match.WinnerID != "" {
		t.Fatalf("reset match=%+v", match)
	}
	states := rt.siege.States()
	if len(states) != 1 || states[0].HP != 100 || states[0].Destroyed {
		t.Fatalf("reset gate=%+v", states)
	}
	if enabled, err := nav.BlockerEnabled("main-gate"); err != nil || !enabled {
		t.Fatalf("reset blocker enabled=%v err=%v", enabled, err)
	}
	if rt.dynamicRevision != beforeDynamic+1 {
		t.Fatalf("dynamic revision=%d want=%d", rt.dynamicRevision, beforeDynamic+1)
	}

	stableRevision := rt.dynamicRevision
	if err := rt.EnqueueStartNextSiegeRound(); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("repeat report=%#v", report)
	}
	if rt.dynamicRevision != stableRevision {
		t.Fatalf("repeat reset changed dynamic revision=%d want=%d", rt.dynamicRevision, stableRevision)
	}
}
