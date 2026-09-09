package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDynamicBlockerStateReplicatesWithRevision(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "dynamic-test", Revision: "r1", Units: "meters",
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
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg, WithDynamicWorld(nav))
	conn := session.NewQueueConnection(16, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }

	first := rt.Step(1, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || len(first.DeliveryErrors) != 0 { t.Fatalf("step1 errors: %#v", first) }
	state1 := nextDynamicState(t, conn)
	if state1.Revision != 1 || len(state1.Blockers) != 1 || state1.Blockers[0].ID != "main-gate" || !state1.Blockers[0].Enabled {
		t.Fatalf("unexpected initial state: %#v", state1)
	}

	if err := rt.EnqueueSetBlocker("main-gate", false); err != nil { t.Fatal(err) }
	second := rt.Step(2, 50*time.Millisecond)
	if len(second.CommandErrors) != 0 || len(second.DeliveryErrors) != 0 { t.Fatalf("step2 errors: %#v", second) }
	state2 := nextDynamicState(t, conn)
	if state2.Revision != 2 || len(state2.Blockers) != 1 || state2.Blockers[0].Enabled {
		t.Fatalf("unexpected updated state: %#v", state2)
	}
	if enabled, err := nav.BlockerEnabled("main-gate"); err != nil || enabled {
		t.Fatalf("navigator state enabled=%v err=%v", enabled, err)
	}
}

func nextDynamicState(t *testing.T, conn *session.QueueConnection) protocol.WorldDynamicState {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope := <-conn.Reliable():
			switch state := envelope.Message.(type) {
			case protocol.WorldDynamicState:
				return state
			case protocol.InventorySnapshot, protocol.EquipmentSnapshot:
				// Protocol v14+ character bootstrap shares the same ReliableOrdered stream.
				// This helper is intentionally scoped to WorldDynamicState, so consume those
				// independent bootstrap views instead of coupling legacy world-state tests to
				// their delivery position.
				continue
			default:
				t.Fatalf("unexpected reliable message: %#v", envelope.Message)
			}
		case <-timer.C:
			t.Fatal("timed out waiting for WorldDynamicState")
			return protocol.WorldDynamicState{}
		}
	}
}
