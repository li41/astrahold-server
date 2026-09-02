package worldruntime

import (
	"reflect"
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

func TestJoinEmitsAuthoritativeInventorySnapshot(t *testing.T) {
	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(16, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session:       s,
		Entity:        world.EntityState{ID: 10, Kind: world.EntityPlayer},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}

	report := runtime.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}

	select {
	case envelope := <-connection.Reliable():
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			t.Fatalf("delivery = %v, want reliable ordered", envelope.Delivery)
		}
		snapshot, ok := envelope.Message.(protocol.InventorySnapshot)
		if !ok {
			t.Fatalf("message = %T, want protocol.InventorySnapshot", envelope.Message)
		}
		want := protocol.InventorySnapshot{
			Revision: 3,
			Items: []protocol.InventoryItemStack{
				{ArchetypeID: "item_minor_healing_potion", Quantity: 5},
				{ArchetypeID: "item_minor_mana_potion", Quantity: 3},
				{ArchetypeID: "item_training_blade", Quantity: 1},
			},
		}
		if !reflect.DeepEqual(snapshot, want) {
			t.Fatalf("snapshot = %#v, want %#v", snapshot, want)
		}
	default:
		t.Fatal("expected reliable inventory snapshot")
	}
}
