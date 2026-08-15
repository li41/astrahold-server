package worldruntime

import (
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

func TestTrustedSiegeRosterDrivesWorldRuntimeCaptureAndContestReset(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "trusted-siege-test",
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
	for _, id := range []world.EntityID{1, 2, 3, 4} {
		if err := sim.Spawn(world.EntityState{
			ID: id, Kind: world.EntityPlayer,
			Transform: world.Transform{Position: inside},
		}, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	attackerIdentity, _ := characteridentity.NewTrusted("character.attacker")
	defenderIdentity, _ := characteridentity.NewTrusted("character.defender")
	unlistedIdentity, _ := characteridentity.NewTrusted("character.unlisted")
	ephemeralIdentity, _ := characteridentity.NewEphemeral()
	throne := siege.ThroneObjectiveDefinition{
		ID: "throne",
		Zone: siege.ObjectiveZone{
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8},
		},
		CaptureDuration: 100 * time.Millisecond,
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
			ParticipantTeams: map[characteridentity.ID]siege.Team{
				attackerIdentity.ID: siege.TeamAttacker,
				defenderIdentity.ID: siege.TeamDefender,
			},
		}),
	)

	register := func(sessionID session.ID, entityID world.EntityID, identity characteridentity.Binding) {
		t.Helper()
		conn := session.NewQueueConnection(64, 64)
		s, err := session.NewWithCharacterIdentity(sessionID, entityID, identity, 20, conn)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(s); err != nil {
			t.Fatal(err)
		}
	}
	register(1, 1, attackerIdentity)
	register(3, 3, unlistedIdentity)
	register(4, 4, ephemeralIdentity)
	if report := rt.Step(1, 40*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register report=%#v", report)
	}
	if team, ok := rt.siege.ParticipantTeam(1); !ok || team != siege.TeamAttacker {
		t.Fatalf("attacker team=%v ok=%v", team, ok)
	}
	if _, ok := rt.siege.ParticipantTeam(3); ok {
		t.Fatal("unlisted trusted identity must remain unknown")
	}
	if _, ok := rt.siege.ParticipantTeam(4); ok {
		t.Fatal("ephemeral identity must remain unknown")
	}

	if !rt.siege.ObserveGateState(siege.GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("expected gate -> throne transition")
	}
	if report := rt.Step(2, 40*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("capture report=%#v", report)
	}
	capture, ok := rt.SiegeThroneCaptureState()
	if !ok || capture.Progress != 40*time.Millisecond || capture.ReadyForResolution {
		t.Fatalf("capture=%+v ok=%v", capture, ok)
	}

	register(2, 2, defenderIdentity)
	if report := rt.Step(3, 40*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("contest report=%#v", report)
	}
	presence, _ := rt.SiegeThronePresenceState()
	capture, _ = rt.SiegeThroneCaptureState()
	if !presence.Contested || presence.CaptureEligible || capture.Progress != 0 || capture.ReadyForResolution {
		t.Fatalf("presence=%+v capture=%+v", presence, capture)
	}

	if err := rt.EnqueueUnregister(2); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 40*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("defender unregister report=%#v", report)
	}
	if _, ok := rt.siege.ParticipantTeam(2); ok {
		t.Fatal("unregister must remove entity team assignment")
	}
	capture, _ = rt.SiegeThroneCaptureState()
	if capture.Progress != 40*time.Millisecond || capture.ReadyForResolution {
		t.Fatalf("capture restart=%+v", capture)
	}

	if report := rt.Step(5, 60*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("ready report=%#v", report)
	}
	capture, _ = rt.SiegeThroneCaptureState()
	if capture.Progress != capture.Required || !capture.ReadyForResolution {
		t.Fatalf("ready capture=%+v", capture)
	}
	match, _ := rt.SiegeMatchState()
	if match.Phase != siege.MatchPhaseThrone || match.Revision != 2 {
		t.Fatalf("D.2B must not resolve match: %+v", match)
	}
}
