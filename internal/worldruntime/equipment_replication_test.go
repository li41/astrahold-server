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
