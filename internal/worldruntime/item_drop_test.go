package worldruntime

import (
	"errors"
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

const testItemDropArchetypeID = "item_test_pelt"

func newItemDropTestRuntime(t *testing.T, playerPosition world.Position) (*Runtime, *simulation.World, *session.Session) {
	t.Helper()
	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{
			ID:        10,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: playerPosition},
		},
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
	return runtime, sim, s
}

func spawnTestItemDrop(t *testing.T, runtime *Runtime, sim *simulation.World, position world.Position) world.EntityID {
	t.Helper()
	dropID, err := runtime.spawnItemDrop(testItemDropArchetypeID, position)
	if err != nil {
		t.Fatal(err)
	}
	drop, ok := sim.Entity(dropID)
	if !ok {
		t.Fatal("expected item drop in authoritative world")
	}
	if drop.Kind != world.EntityItemDrop || drop.ArchetypeID != testItemDropArchetypeID {
		t.Fatalf("drop = %#v", drop)
	}
	return dropID
}

func TestPickupItemAddsInventoryAndRemovesWorldDrop(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	dropID := spawnTestItemDrop(t, runtime, sim, world.Position{X: 1})

	if err := runtime.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: dropID}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("pickup errors: %#v", report.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); ok {
		t.Fatal("picked drop still exists in authoritative world")
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if got := inv.Quantity(testItemDropArchetypeID); got != 1 {
		t.Fatalf("item quantity = %d, want 1", got)
	}
	if got := inv.Revision(); got != 4 {
		t.Fatalf("inventory revision = %d, want 4", got)
	}
}

func TestPickupItemRejectsOutOfRangeWithoutMutation(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	dropID := spawnTestItemDrop(t, runtime, sim, world.Position{X: 10})

	if err := runtime.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: dropID}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrItemDropOutOfRange) {
		t.Fatalf("pickup errors = %#v, want out of range", report.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); !ok {
		t.Fatal("rejected pickup removed authoritative drop")
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if got := inv.Quantity(testItemDropArchetypeID); got != 0 {
		t.Fatalf("item quantity = %d, want 0", got)
	}
	if got := inv.Revision(); got != 3 {
		t.Fatalf("inventory revision = %d, want 3", got)
	}
}
