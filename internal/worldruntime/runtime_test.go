package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func makeRuntime(t *testing.T) (*Runtime, *simulation.World, *session.QueueConnection) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, e := range []world.EntityState{{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, {ID: 2, Kind: world.EntityMonster, Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}}}} {
		if err := sim.Spawn(e, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	conn := session.NewQueueConnection(16, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	rt := New(sim, cfg)
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueMove(1, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	return rt, sim, conn
}
func TestStepProcessesCommandsMovesAndReplicates(t *testing.T) {
	rt, sim, conn := makeRuntime(t)
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.TickErrors) != 0 {
		t.Fatalf("unexpected errors: %#v", report)
	}
	e, _ := sim.Entity(1)
	if e.Transform.Position.X < 0.29 || e.Transform.Position.X > 0.31 {
		t.Fatalf("expected x~0.3, got %f", e.Transform.Position.X)
	}
	reliable := 0
drainReliable:
	for {
		select {
		case <-conn.Reliable():
			reliable++
		default:
			break drainReliable
		}
	}
	if reliable != 2 {
		t.Fatalf("expected two spawn messages, got %d", reliable)
	}
	var gotCorrection bool
drainRealtime:
	for {
		select {
		case env := <-conn.Realtime():
			if c, ok := env.Message.(protocol.PositionCorrection); ok {
				gotCorrection = true
				if c.LastProcessedInputSequence != 1 {
					t.Fatalf("expected ack seq 1, got %d", c.LastProcessedInputSequence)
				}
			}
		default:
			break drainRealtime
		}
	}
	if !gotCorrection {
		t.Fatal("expected correction")
	}
}
func TestLoopUsesFixedTick(t *testing.T) {
	rt, _, _ := makeRuntime(t)
	loop, err := NewLoop(rt, 20)
	if err != nil {
		t.Fatal(err)
	}
	report := loop.Step()
	if report.Tick != 1 || loop.Tick() != 1 {
		t.Fatalf("unexpected tick %d", report.Tick)
	}
}

func TestStaleMoveSequenceIsRejectedPerSession(t *testing.T) {
	rt, _, _ := makeRuntime(t)
	first := rt.Step(1, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 {
		t.Fatalf("unexpected initial errors: %#v", first.CommandErrors)
	}
	if err := rt.EnqueueMove(1, 1, protocol.ClientMoveInput{DirectionX: -1}); err != nil {
		t.Fatal(err)
	}
	second := rt.Step(2, 50*time.Millisecond)
	if len(second.CommandErrors) != 1 {
		t.Fatalf("expected one stale sequence error, got %#v", second.CommandErrors)
	}
	if second.CommandErrors[0].Err != session.ErrStaleInput {
		t.Fatalf("expected ErrStaleInput, got %v", second.CommandErrors[0].Err)
	}
}
