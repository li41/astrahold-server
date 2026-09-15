package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/world"
)

func TestStepFenceKeepsFollowingCommandsForNextStep(t *testing.T) {
	rt, sim, _ := makeRuntime(t)

	reached, err := rt.EnqueueStepFence()
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueTeleport(2, world.Position{X: 10, Layer: 0}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-reached:
		t.Fatal("step fence reached before Step drained the command queue")
	default:
	}

	first := rt.Step(1, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || len(first.TickErrors) != 0 {
		t.Fatalf("unexpected first-step errors: %#v", first)
	}
	select {
	case <-reached:
	default:
		t.Fatal("step fence was not reached by the first Step")
	}
	entity, ok := sim.Entity(2)
	if !ok {
		t.Fatal("expected entity 2")
	}
	if entity.Transform.Position.X != 2 {
		t.Fatalf("command after fence ran in the same Step: x=%f", entity.Transform.Position.X)
	}

	second := rt.Step(2, 50*time.Millisecond)
	if len(second.CommandErrors) != 0 || len(second.TickErrors) != 0 {
		t.Fatalf("unexpected second-step errors: %#v", second)
	}
	entity, ok = sim.Entity(2)
	if !ok {
		t.Fatal("expected entity 2 after teleport")
	}
	if entity.Transform.Position.X != 10 {
		t.Fatalf("expected command after fence on next Step: x=%f", entity.Transform.Position.X)
	}
}
