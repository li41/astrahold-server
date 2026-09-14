package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestV28UniqueMainHandReplicatesExactInstanceAndSkinAffinity(t *testing.T) {
	itemID := withMidTierRestoreCatalog(t)
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(41, 410, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 410, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }
	readOwnerSnapshotBatch(t, connection)

	if err := runtime.characterSkills.restoreAppearance(s.EntityID, appearance.KnightDPelegrini); err != nil { t.Fatal(err) }
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	instance := testMidTierInstance(itemID, iteminstance.ID("item-instance:v28-mid-sword-1"))
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }

	if err := runtime.EnqueueEquipmentInstanceCommand(s.ID, 1, protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationEquip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemInstanceID: string(instance.ID),
	}); err != nil { t.Fatal(err) }
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("equip errors=%#v", report.CommandErrors) }
	equipped := readOwnerSnapshotBatch(t, connection)
	if len(equipped.InventoryInstance.Items) != 0 { t.Fatalf("equipped instance remained in unique inventory=%#v", equipped.InventoryInstance.Items) }
	if len(equipped.Equipment.Slots) != 1 || equipped.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || equipped.Equipment.Slots[0].ItemArchetypeID != itemID { t.Fatalf("archetype equipment view=%#v", equipped.Equipment) }
	if len(equipped.EquipmentInstance.Slots) != 1 { t.Fatalf("unique equipment view=%#v", equipped.EquipmentInstance) }
	slot := equipped.EquipmentInstance.Slots[0]
	if slot.Slot != protocol.EquipmentSlotMainHand || slot.Item.ItemInstanceID != string(instance.ID) || slot.Item.ItemArchetypeID != itemID || len(slot.Item.Affixes) != 1 || slot.Item.Affixes[0].AffixID != string(instance.Affixes[0].ID) || slot.Item.Affixes[0].Strength != instance.Affixes[0].Strength || slot.Item.Affixes[0].Value != instance.Affixes[0].Value {
		t.Fatalf("unique equipment slot=%#v want instance=%#v", slot, instance)
	}
	if equipped.Appearance.SkinID != appearance.KnightDPelegrini || equipped.Appearance.BasicAttackAffinityBonus != 1 { t.Fatalf("equipped appearance=%#v", equipped.Appearance) }

	if err := runtime.EnqueueEquipmentInstanceCommand(s.ID, 2, protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand}); err != nil { t.Fatal(err) }
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("unequip errors=%#v", report.CommandErrors) }
	unequipped := readOwnerSnapshotBatch(t, connection)
	if len(unequipped.InventoryInstance.Items) != 1 || unequipped.InventoryInstance.Items[0].ItemInstanceID != string(instance.ID) { t.Fatalf("unequipped inventory instances=%#v", unequipped.InventoryInstance.Items) }
	if len(unequipped.Equipment.Slots) != 0 || len(unequipped.EquipmentInstance.Slots) != 0 { t.Fatalf("unequipped equipment base=%#v unique=%#v", unequipped.Equipment, unequipped.EquipmentInstance) }
	if unequipped.Appearance.SkinID != appearance.KnightDPelegrini || unequipped.Appearance.BasicAttackAffinityBonus != 0 { t.Fatalf("unequipped appearance=%#v", unequipped.Appearance) }
}
