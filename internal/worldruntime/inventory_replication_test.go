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

	inventoryEnvelope := <-connection.Reliable()
	if inventoryEnvelope.Delivery != protocol.DeliveryReliableOrdered {
		t.Fatalf("delivery = %v, want reliable ordered", inventoryEnvelope.Delivery)
	}
	snapshot, ok := inventoryEnvelope.Message.(protocol.InventorySnapshot)
	if !ok {
		t.Fatalf("message = %T, want protocol.InventorySnapshot", inventoryEnvelope.Message)
	}
	want := protocol.InventorySnapshot{
		Revision:           3,
		CurrentCarryWeight: 16,
		MaxCarryWeight:     100,
		Items: []protocol.InventoryItemStack{
			{ArchetypeID: "item_minor_healing_potion", Quantity: 5},
			{ArchetypeID: "item_minor_mana_potion", Quantity: 3},
			{ArchetypeID: "item_training_blade", Quantity: 1},
		},
	}
	if !reflect.DeepEqual(snapshot, want) {
		t.Fatalf("snapshot = %#v, want %#v", snapshot, want)
	}

	equipmentEnvelope := <-connection.Reliable()
	equipment, ok := equipmentEnvelope.Message.(protocol.EquipmentSnapshot)
	if !ok {
		t.Fatalf("message = %T, want protocol.EquipmentSnapshot", equipmentEnvelope.Message)
	}
	if equipment.Revision != 0 || len(equipment.Slots) != 0 {
		t.Fatalf("equipment bootstrap = %#v, want revision 0 empty slots", equipment)
	}
}

func TestInventorySnapshotCarryWeightIncludesEquippedMainHand(t *testing.T) {
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
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	<-connection.Reliable() // bootstrap InventorySnapshot
	<-connection.Reliable() // bootstrap EquipmentSnapshot

	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("authoritative inventory missing after join")
	}
	before := inv.CurrentWeight()
	if err := inv.EquipMainHand("item_training_blade"); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != before {
		t.Fatalf("equipping changed carried weight: before=%d after=%d", before, inv.CurrentWeight())
	}
	runtime.sessionInventoryPending[s.ID] = struct{}{}

	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip replication errors: %#v", report.CommandErrors)
	}
	inventoryEnvelope := <-connection.Reliable()
	snapshot, ok := inventoryEnvelope.Message.(protocol.InventorySnapshot)
	if !ok {
		t.Fatalf("message = %T, want protocol.InventorySnapshot", inventoryEnvelope.Message)
	}
	if snapshot.CurrentCarryWeight != 16 || snapshot.MaxCarryWeight != 100 {
		t.Fatalf("carry weight = %d/%d, want 16/100", snapshot.CurrentCarryWeight, snapshot.MaxCarryWeight)
	}
	for _, item := range snapshot.Items {
		if item.ArchetypeID == "item_training_blade" {
			t.Fatalf("equipped blade remained in inventory items: %#v", snapshot.Items)
		}
	}

	equipmentEnvelope := <-connection.Reliable()
	equipment, ok := equipmentEnvelope.Message.(protocol.EquipmentSnapshot)
	if !ok {
		t.Fatalf("message = %T, want protocol.EquipmentSnapshot", equipmentEnvelope.Message)
	}
	if len(equipment.Slots) != 1 || equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || equipment.Slots[0].ItemArchetypeID != "item_training_blade" {
		t.Fatalf("equipment snapshot = %#v, want training blade in main hand", equipment)
	}
}
