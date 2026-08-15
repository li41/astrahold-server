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

func TestSiegeCompletionPolicyWaitsForMinHoldAfterDelivery(t *testing.T) {
	rt, _ := newCompletedSiegeRoundPolicyRuntime(t, 100*time.Millisecond, 500*time.Millisecond, false)

	report := rt.Step(2, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedActiveSessions != 1 || report.Metrics.SiegeCompletedPendingDeliveries != 0 || report.Metrics.SiegeCompletedElapsed != 0 || report.Metrics.SiegeRoundResetsScheduled != 0 {
		t.Fatalf("first completed report=%+v", report.Metrics)
	}
	match, _ := rt.SiegeMatchState()
	if match.Phase != siege.MatchPhaseCompleted || match.Round != 1 {
		t.Fatalf("first completed match=%+v", match)
	}

	report = rt.Step(3, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedElapsed != 50*time.Millisecond || report.Metrics.SiegeRoundResetsScheduled != 0 {
		t.Fatalf("pre-min report=%+v", report.Metrics)
	}

	report = rt.Step(4, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedElapsed != 100*time.Millisecond || report.Metrics.SiegeRoundResetsScheduled != 1 || report.Metrics.SiegeRoundResetsForcedByMaxHold != 0 {
		t.Fatalf("scheduled report=%+v", report.Metrics)
	}
	match, _ = rt.SiegeMatchState()
	if match.Phase != siege.MatchPhaseCompleted {
		t.Fatalf("scheduled reset must execute next Step, match=%+v", match)
	}
	if report.Metrics.CommandQueueDepthAfter != 1 {
		t.Fatalf("scheduled command queue depth=%d", report.Metrics.CommandQueueDepthAfter)
	}

	report = rt.Step(5, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("reset report=%#v", report.CommandErrors)
	}
	match, _ = rt.SiegeMatchState()
	if match.Phase != siege.MatchPhaseGate || match.Round != 2 || match.AttackerID != "defenders" || match.DefenderID != "attackers" {
		t.Fatalf("reset match=%+v", match)
	}
}

func TestSiegeCompletionPolicyForcesResetAtMaxHoldUnderBackpressure(t *testing.T) {
	rt, conn := newCompletedSiegeRoundPolicyRuntime(t, 50*time.Millisecond, 150*time.Millisecond, true)

	report := rt.Step(2, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedPendingDeliveries != 1 || report.Metrics.SiegeCompletedElapsed != 0 || report.Metrics.SiegeRoundResetsScheduled != 0 {
		t.Fatalf("first blocked report=%+v", report.Metrics)
	}
	if len(conn.siegeMessages()) != 1 {
		t.Fatalf("completed delivery unexpectedly accepted messages=%#v", conn.siegeMessages())
	}

	report = rt.Step(3, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedElapsed != 50*time.Millisecond || report.Metrics.SiegeCompletedPendingDeliveries != 1 || report.Metrics.SiegeRoundResetsScheduled != 0 {
		t.Fatalf("min blocked report=%+v", report.Metrics)
	}
	report = rt.Step(4, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedElapsed != 100*time.Millisecond || report.Metrics.SiegeRoundResetsScheduled != 0 {
		t.Fatalf("mid blocked report=%+v", report.Metrics)
	}
	report = rt.Step(5, 50*time.Millisecond)
	if report.Metrics.SiegeCompletedElapsed != 150*time.Millisecond || report.Metrics.SiegeCompletedPendingDeliveries != 1 || report.Metrics.SiegeRoundResetsScheduled != 1 || report.Metrics.SiegeRoundResetsForcedByMaxHold != 1 {
		t.Fatalf("forced report=%+v", report.Metrics)
	}

	report = rt.Step(6, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("forced reset report=%#v", report.CommandErrors)
	}
	match, _ := rt.SiegeMatchState()
	if match.Phase != siege.MatchPhaseGate || match.Round != 2 {
		t.Fatalf("forced reset match=%+v", match)
	}
}

func newCompletedSiegeRoundPolicyRuntime(t *testing.T, minHold, maxHold time.Duration, failCompleted bool) (*Runtime, *siegeRecordingConnection) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "round-policy-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
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
		Transform: world.Transform{Position: world.Position{Z: 0, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	throne := siege.ThroneObjectiveDefinition{
		ID:              "throne",
		Zone:            siege.ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8}},
		CaptureDuration: time.Millisecond,
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.SiegeCompletedMinHold = minHold
	cfg.SiegeCompletedMaxHold = maxHold
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
	conn := &siegeRecordingConnection{}
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("register report=%#v", report)
	}

	gateState, err := rt.siege.Attack(1, world.Position{Z: 0, Layer: 0}, "main-gate", 1, 50*time.Millisecond, nav)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.siege.ObserveGateState(gateState) {
		t.Fatal("expected gate breach")
	}
	if !rt.siege.ObserveThronePresence([]siege.ParticipantPresence{{EntityID: 1, Position: world.Position{Z: 6, Layer: 0}}}) {
		t.Fatal("expected throne presence")
	}
	if !rt.siege.AdvanceThroneCapture(time.Millisecond) {
		t.Fatal("expected capture readiness")
	}
	if !rt.siege.ResolveThroneCapture() {
		t.Fatal("expected completed round")
	}
	conn.failSiege = failCompleted
	return rt, conn
}
