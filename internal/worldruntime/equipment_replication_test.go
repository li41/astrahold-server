package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/appearance"
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
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	join := readOwnerSnapshotBatch(t, connection)
	if join.Inventory.Revision != 3 || join.Equipment.Revision != 0 || len(join.InventoryInstance.Items) != 0 || len(join.EquipmentInstance.Slots) != 0 {
		t.Fatalf("owner bootstrap=%#v", join)
	}
	if join.Appearance.SkinID != appearance.None || join.Appearance.BasicAttackAffinityBonus != 0 {
		t.Fatalf("appearance at join=%#v", join.Appearance)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_training_blade"}); err != nil { t.Fatal(err) }
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("equip errors: %#v", report.CommandErrors) }
	afterEquip := readOwnerSnapshotBatch(t, connection)
	if afterEquip.Inventory.Revision != 4 { t.Fatalf("inventory revision after equip=%d want=4", afterEquip.Inventory.Revision) }
	for _, stack := range afterEquip.Inventory.Items {
		if stack.ArchetypeID == "item_training_blade" { t.Fatalf("training blade remained in inventory after equip: %#v", stack) }
	}
	if len(afterEquip.InventoryInstance.Items) != 0 || len(afterEquip.EquipmentInstance.Slots) != 0 {
		t.Fatalf("low-tier equip leaked unique-instance state: inventory=%#v equipment=%#v", afterEquip.InventoryInstance, afterEquip.EquipmentInstance)
	}
	if afterEquip.Equipment.Revision != 1 || len(afterEquip.Equipment.Slots) != 1 || afterEquip.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || afterEquip.Equipment.Slots[0].ItemArchetypeID != "item_training_blade" {
		t.Fatalf("equipment after equip=%#v", afterEquip.Equipment)
	}
	if afterEquip.Appearance.SkinID != appearance.None || afterEquip.Appearance.BasicAttackAffinityBonus != 0 {
		t.Fatalf("appearance after training-blade equip=%#v", afterEquip.Appearance)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 2, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand}); err != nil { t.Fatal(err) }
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("unequip errors: %#v", report.CommandErrors) }
	afterUnequip := readOwnerSnapshotBatch(t, connection)
	if afterUnequip.Inventory.Revision != 5 { t.Fatalf("inventory revision after unequip=%d want=5", afterUnequip.Inventory.Revision) }
	bladeQuantity := uint32(0)
	for _, stack := range afterUnequip.Inventory.Items { if stack.ArchetypeID == "item_training_blade" { bladeQuantity = stack.Quantity } }
	if bladeQuantity != 1 { t.Fatalf("training blade quantity after unequip=%d want=1", bladeQuantity) }
	if afterUnequip.Equipment.Revision != 2 || len(afterUnequip.Equipment.Slots) != 0 { t.Fatalf("equipment after unequip=%#v", afterUnequip.Equipment) }
	if len(afterUnequip.InventoryInstance.Items) != 0 || len(afterUnequip.EquipmentInstance.Slots) != 0 { t.Fatalf("unique state after low-tier unequip=%#v", afterUnequip) }
	if afterUnequip.Appearance.SkinID != appearance.None || afterUnequip.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("appearance after unequip=%#v", afterUnequip.Appearance) }
}

func TestEquipmentSnapshotReplicatesMainAndOffHandTogether(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(2, 20, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 20, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	readOwnerSnapshotBatch(t, connection)

	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.Add("item_militia_iron_sword", 1); err != nil { t.Fatal(err) }
	if err := inv.Add("item_runed_square_shield", 1); err != nil { t.Fatal(err) }

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_militia_iron_sword"}); err != nil { t.Fatal(err) }
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("main-hand equip errors: %#v", report.CommandErrors) }
	mainOnly := readOwnerSnapshotBatch(t, connection)
	if len(mainOnly.Equipment.Slots) != 1 || mainOnly.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || mainOnly.Equipment.Slots[0].ItemArchetypeID != "item_militia_iron_sword" { t.Fatalf("main-only equipment=%#v", mainOnly.Equipment) }
	if len(mainOnly.InventoryInstance.Items) != 0 || len(mainOnly.EquipmentInstance.Slots) != 0 { t.Fatalf("main-only unique state=%#v", mainOnly) }
	if mainOnly.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("main-only appearance=%#v", mainOnly.Appearance) }

	if err := runtime.EnqueueEquipmentCommand(s.ID, 2, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotOffHand, ItemArchetypeID: "item_runed_square_shield"}); err != nil { t.Fatal(err) }
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("off-hand equip errors: %#v", report.CommandErrors) }
	both := readOwnerSnapshotBatch(t, connection)
	if len(both.Equipment.Slots) != 2 { t.Fatalf("equipment slots=%#v", both.Equipment.Slots) }
	if both.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || both.Equipment.Slots[0].ItemArchetypeID != "item_militia_iron_sword" { t.Fatalf("main-hand slot=%#v", both.Equipment.Slots[0]) }
	if both.Equipment.Slots[1].Slot != protocol.EquipmentSlotOffHand || both.Equipment.Slots[1].ItemArchetypeID != "item_runed_square_shield" { t.Fatalf("off-hand slot=%#v", both.Equipment.Slots[1]) }
	if got := both.Inventory.CurrentCarryWeight; got != inv.CurrentWeight() { t.Fatalf("snapshot carry weight=%d runtime=%d", got, inv.CurrentWeight()) }
	if inv.MainHand() != "item_militia_iron_sword" || inv.OffHand() != "item_runed_square_shield" { t.Fatalf("runtime equipment main=%q off=%q", inv.MainHand(), inv.OffHand()) }
	if len(both.InventoryInstance.Items) != 0 || len(both.EquipmentInstance.Slots) != 0 { t.Fatalf("low-tier both-slot unique state=%#v", both) }
	if both.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("both-slot appearance=%#v", both.Appearance) }
}

func TestAppearanceSnapshotTracksMatchingSkinBonusAcrossMainHandChange(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(3, 30, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 30, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	readOwnerSnapshotBatch(t, connection)

	if err := runtime.characterSkills.restoreAppearance(s.EntityID, appearance.KnightDPelegrini); err != nil { t.Fatal(err) }
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.Add("item_militia_iron_sword", 1); err != nil { t.Fatal(err) }

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_militia_iron_sword"}); err != nil { t.Fatal(err) }
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("equip errors: %#v", report.CommandErrors) }
	matching := readOwnerSnapshotBatch(t, connection)
	if matching.Appearance.SkinID != appearance.KnightDPelegrini || matching.Appearance.BasicAttackAffinityBonus != 1 { t.Fatalf("matching appearance=%#v want skin=%q bonus=1", matching.Appearance, appearance.KnightDPelegrini) }

	if err := runtime.EnqueueEquipmentCommand(s.ID, 2, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand}); err != nil { t.Fatal(err) }
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("unequip errors: %#v", report.CommandErrors) }
	withoutWeapon := readOwnerSnapshotBatch(t, connection)
	if withoutWeapon.Appearance.SkinID != appearance.KnightDPelegrini || withoutWeapon.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("appearance without weapon=%#v want skin=%q bonus=0", withoutWeapon.Appearance, appearance.KnightDPelegrini) }
}
