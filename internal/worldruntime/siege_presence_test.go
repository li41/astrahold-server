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

func TestWorldRuntimeObservesAuthoritativeThronePresenceAndDefeat(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "throne-presence-test",
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
	inside := world.Position{X: 0, Y: 0, Z: 6, Layer: 0}
	for _, id := range []world.EntityID{1, 2} {
		if err := sim.Spawn(world.EntityState{
			ID: id, Kind: world.EntityPlayer,
			Transform: world.Transform{Position: inside},
		}, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	throne := siege.ThroneObjectiveDefinition{
		ID: "throne",
		Zone: siege.ObjectiveZone{
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8},
		},
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
	for _, id := range []session.ID{1, 2} {
		conn := session.NewQueueConnection(64, 64)
		s, err := session.New(id, world.EntityID(id), 20, conn)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(s); err != nil {
			t.Fatal(err)
		}
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register report=%#v", report)
	}
	if err := rt.siege.AssignParticipant(1, siege.TeamAttacker); err != nil {
		t.Fatal(err)
	}
	if err := rt.siege.AssignParticipant(2, siege.TeamDefender); err != nil {
		t.Fatal(err)
	}
	if !rt.siege.ObserveGateState(siege.GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("expected gate -> throne transition")
	}

	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("contest report=%#v", report)
	}
	contested, ok := rt.SiegeThronePresenceState()
	if !ok || !contested.Active || contested.AttackerCount != 1 || contested.DefenderCount != 1 || !contested.Contested || contested.CaptureEligible {
		t.Fatalf("contested=%+v ok=%v", contested, ok)
	}

	if _, err := rt.characters.ApplyDamage(2, 1000); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("defeat report=%#v", report)
	}
	eligible, _ := rt.SiegeThronePresenceState()
	if eligible.AttackerCount != 1 || eligible.DefenderCount != 0 || eligible.Contested || !eligible.CaptureEligible {
		t.Fatalf("eligible=%+v", eligible)
	}

	if err := rt.EnqueueTeleport(1, world.Position{X: 0, Y: 0, Z: -5, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave zone report=%#v", report)
	}
	empty, _ := rt.SiegeThronePresenceState()
	if empty.AttackerCount != 0 || empty.DefenderCount != 0 || empty.Contested || empty.CaptureEligible {
		t.Fatalf("empty=%+v", empty)
	}
}
