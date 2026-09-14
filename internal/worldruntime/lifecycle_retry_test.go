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

func TestLifecycleBackpressureRetriesWithoutDeliveryLoss(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	// Protocol v28 registration emits five independent authoritative ReliableOrdered owner views
	// before lifecycle replication. Capacity 6 therefore admits those five plus EntitySpawn(1),
	// deliberately forcing EntitySpawn(2) to backpressure so retry semantics remain exact.
	conn := session.NewQueueConnection(6, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	rt := New(sim, cfg)
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}

	first := rt.Step(1, 50*time.Millisecond)
	if len(first.DeliveryErrors) != 0 {
		t.Fatalf("lifecycle backpressure must be deferred, not permanent delivery error: %#v", first.DeliveryErrors)
	}
	if !rt.replication.Knows(1, 1) {
		t.Fatal("first spawn should be confirmed after successful TrySend")
	}
	if rt.replication.Knows(1, 2) {
		t.Fatal("backpressured spawn must not become known")
	}

	wantBootstrap := []protocol.MessageType{
		protocol.MessageInventorySnapshot,
		protocol.MessageInventoryInstanceSnapshot,
		protocol.MessageEquipmentSnapshot,
		protocol.MessageEquipmentInstanceSnapshot,
		protocol.MessageAppearanceSnapshot,
	}
	for index, want := range wantBootstrap {
		select {
		case envelope := <-conn.Reliable():
			if got := envelope.Message.Type(); got != want {
				t.Fatalf("bootstrap[%d] type=%d (%T) want=%d", index, got, envelope.Message, want)
			}
		default:
			t.Fatalf("expected bootstrap[%d] type=%d in reliable queue", index, want)
		}
	}
	select {
	case envelope := <-conn.Reliable():
		spawn, ok := envelope.Message.(protocol.EntitySpawn)
		if !ok || spawn.EntityID != 1 {
			t.Fatalf("sixth reliable message=%T %#v, want EntitySpawn(1)", envelope.Message, envelope.Message)
		}
	default:
		t.Fatal("expected first spawn in reliable queue")
	}

	second := rt.Step(2, 50*time.Millisecond)
	if len(second.DeliveryErrors) != 0 {
		t.Fatalf("retry should not report delivery error: %#v", second.DeliveryErrors)
	}
	if !rt.replication.Knows(1, 2) {
		t.Fatal("second entity spawn was not retried and confirmed")
	}
	select {
	case envelope := <-conn.Reliable():
		spawn, ok := envelope.Message.(protocol.EntitySpawn)
		if !ok || spawn.EntityID != 2 {
			t.Fatalf("retried reliable message=%T %#v, want EntitySpawn(2)", envelope.Message, envelope.Message)
		}
	default:
		t.Fatal("expected retried second spawn in reliable queue")
	}
}
