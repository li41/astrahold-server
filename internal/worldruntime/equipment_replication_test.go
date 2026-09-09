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

func TestEquipmentCommandMovesTrainingBladeAndReplicatesAuthoritativeTruth(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	<-connection.Reliable() // Inventory revision 3.
	<-connection.Reliable() // Equipment revision 0.

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_training_blade"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip errors: %#v", report.CommandErrors)
	}

	inventoryAfterEquip := (<-connection.Reliable()).Message.(protocol.InventorySnapshot)
	if inventoryAfterEquip.Revision != 4 {
		t.Fatalf("inventory revision after equip=%d want=4", inventoryAfterEquip.Revision)
	}
	for _, stack := range inventoryAfterEquip.Items {
		if stack.ArchetypeID == "item_training_blade" {
			t.Fatalf("training blade remained in inventory after equip: %#v", stack)
		}
	}
	equipmentAfterEquip := (<-connection.Reliable()).Message.(protocol.EquipmentSnapshot)
	if equipmentAfterEquip.Revision != 1 || len(equipmentAfterEquip.Slots) != 1 || equipmentAfterEquip.Slots[0].Slot != protocol.EquipmentSlotMainHand || equipmentAfterEquip.Slots[0].ItemArchetypeID != "item_training_blade" {
		t.Fatalf("equipment after equip=%#v", equipmentAfterEquip)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 2, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("unequip errors: %#v", report.CommandErrors)
	}

	inventoryAfterUnequip := (<-connection.Reliable()).Message.(protocol.InventorySnapshot)
	if inventoryAfterUnequip.Revision != 5 {
		t.Fatalf("inventory revision after unequip=%d want=5", inventoryAfterUnequip.Revision)
	}
	bladeQuantity := uint32(0)
	for _, stack := range inventoryAfterUnequip.Items {
		if stack.ArchetypeID == "item_training_blade" {
			bladeQuantity = stack.Quantity
		}
	}
	if bladeQuantity != 1 {
		t.Fatalf("training blade quantity after unequip=%d want=1", bladeQuantity)
	}
	equipmentAfterUnequip := (<-connection.Reliable()).Message.(protocol.EquipmentSnapshot)
	if equipmentAfterUnequip.Revision != 2 || len(equipmentAfterUnequip.Slots) != 0 {
		t.Fatalf("equipment after unequip=%#v", equipmentAfterUnequip)
	}
}

func TestEquipmentSnapshotReplicatesMainAndOffHandTogether(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(2, 20, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 20, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	<-connection.Reliable()
	<-connection.Reliable()

	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add("item_militia_iron_sword", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.Add("item_runed_square_shield", 1); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_militia_iron_sword"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("main-hand equip errors: %#v", report.CommandErrors)
	}
	<-connection.Reliable() // Inventory.
	mainOnly := (<-connection.Reliable()).Message.(protocol.EquipmentSnapshot)
	if len(mainOnly.Slots) != 1 || mainOnly.Slots[0].Slot != protocol.EquipmentSlotMainHand || mainOnly.Slots[0].ItemArchetypeID != "item_militia_iron_sword" {
		t.Fatalf("main-only equipment=%#v", mainOnly)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 2, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotOffHand, ItemArchetypeID: "item_runed_square_shield"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("off-hand equip errors: %#v", report.CommandErrors)
	}
	inventoryAfterBoth := (<-connection.Reliable()).Message.(protocol.InventorySnapshot)
	equipmentAfterBoth := (<-connection.Reliable()).Message.(protocol.EquipmentSnapshot)
	if len(equipmentAfterBoth.Slots) != 2 {
		t.Fatalf("equipment slots=%#v", equipmentAfterBoth.Slots)
	}
	if equipmentAfterBoth.Slots[0].Slot != protocol.EquipmentSlotMainHand || equipmentAfterBoth.Slots[0].ItemArchetypeID != "item_militia_iron_sword" {
		t.Fatalf("main-hand slot=%#v", equipmentAfterBoth.Slots[0])
	}
	if equipmentAfterBoth.Slots[1].Slot != protocol.EquipmentSlotOffHand || equipmentAfterBoth.Slots[1].ItemArchetypeID != "item_runed_square_shield" {
		t.Fatalf("off-hand slot=%#v", equipmentAfterBoth.Slots[1])
	}
	if got := inventoryAfterBoth.CurrentCarryWeight; got != inv.CurrentWeight() {
		t.Fatalf("snapshot carry weight=%d runtime=%d", got, inv.CurrentWeight())
	}
	if inv.MainHand() != "item_militia_iron_sword" || inv.OffHand() != "item_runed_square_shield" {
		t.Fatalf("runtime equipment main=%q off=%q", inv.MainHand(), inv.OffHand())
	}
}
