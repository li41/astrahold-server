package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestWorldRuntimeDurableSiegeOwnershipBarrierRetriesBeforeCompleted(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "durable-siege-test",
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
	inside := world.Position{Z: 6, Layer: 0}
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: inside}}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	attackerIdentity, _ := characteridentity.NewTrusted("character.attacker")
	throne := siege.ThroneObjectiveDefinition{
		ID:              "throne",
		Zone:            siege.ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8}},
		CaptureDuration: 40 * time.Millisecond,
	}
	commitErr := errors.New("ownership disk unavailable")
	attempts := 0
	committer := func(transfer siege.CastleOwnershipTransfer) (siege.CastleOwnershipState, error) {
		attempts++
		if attempts == 1 {
			return siege.CastleOwnershipState{}, commitErr
		}
		return siege.CastleOwnershipState{Revision: 6, OwnerID: "attackers", PreviousOwnerID: "defenders", LastTransferMatchID: "m1"}, nil
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
			ParticipantTeams: map[characteridentity.ID]siege.Team{attackerIdentity.ID: siege.TeamAttacker},
		}),
		WithSiegeOwnershipPersistence(siege.CastleOwnershipState{Revision: 5, OwnerID: "defenders", PreviousOwnerID: "older", LastTransferMatchID: "m0"}, committer),
	)
	conn := session.NewQueueConnection(64, 64)
	s, err := session.NewWithCharacterIdentity(1, 1, attackerIdentity, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 20*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register report=%#v", report)
	}
	if !rt.siege.ObserveGateState(siege.GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("expected gate breach")
	}

	rt.Step(2, 40*time.Millisecond)
	match, _ := rt.SiegeMatchState()
	ownership, _ := rt.SiegeCastleOwnershipState()
	capture, _ := rt.SiegeThroneCaptureState()
	if match.Phase != siege.MatchPhaseThrone || match.WinnerTeam != siege.TeamUnknown || ownership.Revision != 5 || ownership.OwnerID != "defenders" || !capture.ReadyForResolution {
		t.Fatalf("failed durable barrier match=%+v ownership=%+v capture=%+v", match, ownership, capture)
	}

	rt.Step(3, 20*time.Millisecond)
	match, _ = rt.SiegeMatchState()
	ownership, _ = rt.SiegeCastleOwnershipState()
	if match.Phase != siege.MatchPhaseCompleted || match.WinnerTeam != siege.TeamAttacker || ownership.Revision != 6 || ownership.OwnerID != "attackers" {
		t.Fatalf("retry match=%+v ownership=%+v", match, ownership)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}
