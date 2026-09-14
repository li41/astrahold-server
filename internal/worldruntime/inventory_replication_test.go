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
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(16, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }

	batch := readOwnerSnapshotBatch(t, connection)
	want := protocol.InventorySnapshot{
		Revision: 3, CurrentCarryWeight: 16, MaxCarryWeight: 100,
		Items: []protocol.InventoryItemStack{
			{ArchetypeID: "item_minor_healing_potion", Quantity: 5},
			{ArchetypeID: "item_minor_mana_potion", Quantity: 3},
			{ArchetypeID: "item_training_blade", Quantity: 1},
		},
	}
	if !reflect.DeepEqual(batch.Inventory, want) { t.Fatalf("snapshot = %#v, want %#v", batch.Inventory, want) }
	if batch.InventoryInstance.Revision != batch.Inventory.Revision || len(batch.InventoryInstance.Items) != 0 { t.Fatalf("inventory instance bootstrap=%#v", batch.InventoryInstance) }
	if batch.Equipment.Revision != 0 || len(batch.Equipment.Slots) != 0 { t.Fatalf("equipment bootstrap=%#v", batch.Equipment) }
	if batch.EquipmentInstance.Revision != batch.Equipment.Revision || len(batch.EquipmentInstance.Slots) != 0 { t.Fatalf("equipment instance bootstrap=%#v", batch.EquipmentInstance) }
	if batch.Appearance.SkinID != "" || batch.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("appearance bootstrap=%#v", batch.Appearance) }
}

func TestInventorySnapshotCarryWeightIncludesEquippedMainHand(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(16, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	readOwnerSnapshotBatch(t, connection)

	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("authoritative inventory missing after join") }
	before := inv.CurrentWeight()
	if err := inv.EquipMainHand("item_training_blade"); err != nil { t.Fatal(err) }
	if inv.CurrentWeight() != before { t.Fatalf("equipping changed carried weight: before=%d after=%d", before, inv.CurrentWeight()) }
	runtime.sessionInventoryPending[s.ID] = struct{}{}

	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("equip replication errors: %#v", report.CommandErrors) }
	batch := readOwnerSnapshotBatch(t, connection)
	if batch.Inventory.CurrentCarryWeight != 16 || batch.Inventory.MaxCarryWeight != 100 { t.Fatalf("carry weight = %d/%d, want 16/100", batch.Inventory.CurrentCarryWeight, batch.Inventory.MaxCarryWeight) }
	for _, item := range batch.Inventory.Items { if item.ArchetypeID == "item_training_blade" { t.Fatalf("equipped blade remained in inventory items: %#v", batch.Inventory.Items) } }
	if len(batch.InventoryInstance.Items) != 0 { t.Fatalf("unexpected unique inventory=%#v", batch.InventoryInstance) }
	if len(batch.Equipment.Slots) != 1 || batch.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || batch.Equipment.Slots[0].ItemArchetypeID != "item_training_blade" { t.Fatalf("equipment snapshot=%#v", batch.Equipment) }
	if len(batch.EquipmentInstance.Slots) != 0 { t.Fatalf("unexpected unique equipment=%#v", batch.EquipmentInstance) }
	if batch.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("appearance affinity bonus=%d want=0", batch.Appearance.BasicAttackAffinityBonus) }
}
