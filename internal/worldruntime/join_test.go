package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestJoinAndLeaveOwnWorldMutation(t *testing.T) {
	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	runtime := New(sim, DefaultConfig())
	connection := session.NewQueueConnection(8, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	request := JoinRequest{
		Session:       s,
		Entity:        world.EntityState{ID: 10, Kind: world.EntityPlayer},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
	}
	if err := runtime.EnqueueJoin(request); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	if _, ok := sim.Entity(10); !ok {
		t.Fatal("entity not spawned")
	}

	if err := runtime.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	report = runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors: %#v", report.CommandErrors)
	}
	if _, ok := sim.Entity(10); ok {
		t.Fatal("entity not removed")
	}
}
