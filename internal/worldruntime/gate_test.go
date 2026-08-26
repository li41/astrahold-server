package worldruntime

import (
	"errors"
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

func TestGateAttackUpdatesHPAndOpensBlocker(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "gate-test", Revision: "r1", Units: "meters",
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
			ID: "main-gate", BlockerID: "main-gate", MaxHP: 200,
			Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 100, CooldownSeconds: 0.5},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg, WithDynamicWorld(nav), WithSiegeGates(definition.Gates))
	conn := session.NewQueueConnection(16, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }

	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 { t.Fatalf("initial errors=%#v", initial.CommandErrors) }
	state1 := nextDynamicState(t, conn)
	if len(state1.Gates) != 1 || state1.Gates[0].HP != 200 || state1.Gates[0].Destroyed {
		t.Fatalf("initial gate=%#v", state1.Gates)
	}

	if err := rt.EnqueueAttackGate(1, 1, "main-gate"); err != nil { t.Fatal(err) }
	first := rt.Step(2, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || len(first.ActionRejections) != 0 { t.Fatalf("first report=%#v", first) }
	started1 := nextActionStarted(t, conn)
	if started1.ActionID != legacyGateActionID || started1.TargetKind != protocol.ActionTargetGate || started1.TargetID != "main-gate" { t.Fatalf("first start=%#v", started1) }
	state2 := nextDynamicState(t, conn)
	if state2.Revision != 2 || state2.Gates[0].HP != 100 || state2.Gates[0].Destroyed || !state2.Blockers[0].Enabled {
		t.Fatalf("after first attack=%#v", state2)
	}

	if err := rt.EnqueueAttackGate(1, 2, "main-gate"); err != nil { t.Fatal(err) }
	cooldown := rt.Step(3, 50*time.Millisecond)
	if len(cooldown.CommandErrors) != 0 || len(cooldown.ActionRejections) != 1 || !errors.Is(cooldown.ActionRejections[0].Err, siege.ErrGateAttackCooldown) {
		t.Fatalf("cooldown report=%#v", cooldown)
	}
	rejected := nextActionRejected(t, conn)
	if rejected.ClientActionSequence != 2 || rejected.ActionID != legacyGateActionID || rejected.TargetKind != protocol.ActionTargetGate || rejected.Reason != protocol.ActionRejectionCooldown || rejected.CooldownReadyTick != 12 {
		t.Fatalf("cooldown rejection=%#v", rejected)
	}

	if err := rt.EnqueueAttackGate(1, 3, "main-gate"); err != nil { t.Fatal(err) }
	destroy := rt.Step(12, 50*time.Millisecond)
	if len(destroy.CommandErrors) != 0 || len(destroy.ActionRejections) != 0 { t.Fatalf("destroy report=%#v", destroy) }
	started2 := nextActionStarted(t, conn)
	if started2.ActionID != legacyGateActionID || started2.TargetID != "main-gate" || started2.ActionInstanceID == started1.ActionInstanceID { t.Fatalf("destroy start=%#v first=%#v", started2, started1) }
	state3 := nextDynamicState(t, conn)
	if state3.Revision != 3 || state3.Gates[0].HP != 0 || !state3.Gates[0].Destroyed || state3.Blockers[0].Enabled {
		t.Fatalf("destroyed state=%#v", state3)
	}
	if enabled, err := nav.BlockerEnabled("main-gate"); err != nil || enabled {
		t.Fatalf("blocker enabled=%v err=%v", enabled, err)
	}
}

func nextActionStarted(t *testing.T, conn *session.QueueConnection) protocol.ActionStarted {
	t.Helper()
	select {
	case env := <-conn.Reliable():
		started, ok := env.Message.(protocol.ActionStarted)
		if !ok { t.Fatalf("expected ActionStarted, got %#v", env.Message) }
		return started
	default:
		t.Fatal("missing ActionStarted")
		return protocol.ActionStarted{}
	}
}

func nextActionRejected(t *testing.T, conn *session.QueueConnection) protocol.ActionRejected {
	t.Helper()
	select {
	case env := <-conn.Reliable():
		rejected, ok := env.Message.(protocol.ActionRejected)
		if !ok { t.Fatalf("expected ActionRejected, got %#v", env.Message) }
		return rejected
	default:
		t.Fatal("missing ActionRejected")
		return protocol.ActionRejected{}
	}
}
