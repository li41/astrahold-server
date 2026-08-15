package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSiegeResultReplicationIncludesRoundWinnerAndCastleOwner(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "siege-result-test",
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
			ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000,
			Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 100, CooldownSeconds: 0.5},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{Z:6,Layer:0}}}, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	throne := siege.ThroneObjectiveDefinition{
		ID: "throne",
		Zone: siege.ObjectiveZone{Layer:0, Bounds:gameplayworld.BoundsXZ{MinX:-2,MaxX:2,MinZ:4,MaxZ:8}},
		CaptureDuration: time.Millisecond,
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.SiegeCompletedMinHold = 0
	cfg.SiegeCompletedMaxHold = 0
	rt := New(sim, cfg,
		WithSiegeGates(definition.Gates),
		WithSiegeMatch(siege.MatchDefinition{ID:"castle-sandbox-siege",AttackerID:"attackers",DefenderID:"defenders",BreachGateID:"main-gate",ThroneObjectiveID:"throne",Throne:&throne}),
	)
	if err := rt.siege.AssignParticipant(1, siege.TeamAttacker); err != nil { t.Fatal(err) }
	conn := &siegeRecordingConnection{}
	s, err := session.New(1, 1, 20, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }
	first := rt.Step(1, 50*time.Millisecond)
	if len(first.DeliveryErrors) != 0 { t.Fatalf("first delivery errors=%#v", first.DeliveryErrors) }
	messages := conn.siegeMessages()
	if len(messages) != 1 { t.Fatalf("initial siege messages=%#v", messages) }
	initial := messages[0]
	if initial.Round != 1 || initial.Phase != protocol.SiegePhaseGate || initial.WinnerTeam != protocol.SiegeTeamUnknown || initial.WinnerID != "" || initial.CastleOwnerID != "defenders" {
		t.Fatalf("initial siege result view=%+v", initial)
	}

	if !rt.siege.ObserveGateState(siege.GateState{ID:"main-gate",HP:0,MaxHP:1000,Destroyed:true}) { t.Fatal("expected gate breach") }
	rt.siege.ObserveThronePresence([]siege.ParticipantPresence{{EntityID:1,Position:world.Position{Z:6,Layer:0}}})
	rt.siege.AdvanceThroneCapture(time.Millisecond)
	if !rt.siege.ResolveThroneCapture() { t.Fatal("expected throne resolution") }

	second := rt.Step(2, 50*time.Millisecond)
	if len(second.DeliveryErrors) != 0 { t.Fatalf("completed delivery errors=%#v", second.DeliveryErrors) }
	messages = conn.siegeMessages()
	if len(messages) != 2 { t.Fatalf("completed siege messages=%#v", messages) }
	completed := messages[1]
	if completed.Round != 1 || completed.Phase != protocol.SiegePhaseCompleted || completed.WinnerTeam != protocol.SiegeTeamAttacker || completed.WinnerID != "attackers" || completed.CastleOwnerID != "attackers" {
		t.Fatalf("completed siege result view=%+v", completed)
	}
}
