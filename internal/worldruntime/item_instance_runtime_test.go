package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func testMidTierInstance(itemID string, id iteminstance.ID) iteminstance.Instance {
	return iteminstance.Instance{
		ID:              id,
		ItemArchetypeID: itemID,
		Affixes: []equipmentaffix.Affix{{
			ID: equipmentaffix.AffixPhysicalDamage, Strength: 2, Value: 2,
		}},
	}
}

func TestEquipmentInstanceCommandUsesAuthoritativeInstanceIdentity(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(31, 310, 32, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 310, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	<-connection.Reliable()
	<-connection.Reliable()

	itemID := withMidTierRestoreCatalog(t)
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	instance := testMidTierInstance(itemID, "item-instance:runtime-mid-1")
	definition, ok := defaultEquipmentCatalog.Resolve(itemID)
	if !ok { t.Fatal("test definition missing") }
	if err := iteminstance.Validate(instance, definition); err != nil { t.Fatal(err) }
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }

	intent := protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationEquip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemInstanceID: string(instance.ID),
	}
	if err := runtime.EnqueueEquipmentInstanceCommand(s.ID, 1, intent); err != nil { t.Fatal(err) }
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("equip errors: %#v", report.CommandErrors) }

	got, ok := inv.MainHandInstance()
	if !ok { t.Fatal("unique main hand missing") }
	if got.ID != instance.ID || got.ItemArchetypeID != instance.ItemArchetypeID || len(got.Affixes) != 1 || got.Affixes[0] != instance.Affixes[0] {
		t.Fatalf("equipped instance changed: got=%#v want=%#v", got, instance)
	}
	if _, ok := inv.Instance(instance.ID); ok { t.Fatal("equipped instance remained in unequipped inventory") }
}

func TestEquipmentInstanceCommandRejectsWrongSlotAndUnknownInstance(t *testing.T) {
	itemID := withMidTierRestoreCatalog(t)
	inv := newCharacterInventory(16)
	instance := testMidTierInstance(itemID, "item-instance:wrong-slot-mid-1")
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }

	runtime := &Runtime{}
	if err := runtime.applyEquipInstance(inv, protocol.EquipmentSlotOffHand, instance.ID); !errors.Is(err, ErrEquipmentItemNotAllowed) {
		t.Fatalf("wrong-slot err=%v", err)
	}
	if err := runtime.applyEquipInstance(inv, protocol.EquipmentSlotMainHand, "item-instance:missing"); !errors.Is(err, inventory.ErrInstanceNotFound) {
		t.Fatalf("missing-instance err=%v", err)
	}
}

func TestEquipmentInstanceIntentRequiresExactIdentityOnlyForEquip(t *testing.T) {
	if err := validateEquipmentInstanceIntent(protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationEquip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemInstanceID: " item-instance:bad ",
	}); err == nil {
		t.Fatal("whitespace-padded instance identity accepted")
	}
	if err := validateEquipmentInstanceIntent(protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationUnequip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemInstanceID: "item-instance:must-not-be-sent",
	}); err == nil {
		t.Fatal("unequip accepted client-specified instance identity")
	}
	if err := validateEquipmentInstanceIntent(protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationUnequip,
		Slot: protocol.EquipmentSlotMainHand,
	}); err != nil {
		t.Fatalf("slot-only unequip rejected: %v", err)
	}
}

func TestStagedInstanceSnapshotBuildersPreserveIdentityAffixesAndRevisions(t *testing.T) {
	itemID := withMidTierRestoreCatalog(t)
	inv := newCharacterInventory(16)
	instance := testMidTierInstance(itemID, "item-instance:snapshot-mid-1")
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }

	inventorySnapshot, err := buildInventoryInstanceSnapshot(inv)
	if err != nil { t.Fatal(err) }
	if inventorySnapshot.Revision != inv.Revision() || len(inventorySnapshot.Items) != 1 {
		t.Fatalf("inventory instance snapshot=%#v", inventorySnapshot)
	}
	item := inventorySnapshot.Items[0]
	if item.ItemInstanceID != string(instance.ID) || item.ItemArchetypeID != itemID || len(item.Affixes) != 1 || item.Affixes[0].AffixID != string(instance.Affixes[0].ID) || item.Affixes[0].Strength != 2 || item.Affixes[0].Value != 2 {
		t.Fatalf("inventory instance item=%#v", item)
	}

	if err := (&Runtime{}).applyEquipInstance(inv, protocol.EquipmentSlotMainHand, instance.ID); err != nil { t.Fatal(err) }
	equipmentSnapshot, err := buildEquipmentInstanceSnapshot(inv)
	if err != nil { t.Fatal(err) }
	if equipmentSnapshot.Revision != inv.EquipmentRevision() || len(equipmentSnapshot.Slots) != 1 || equipmentSnapshot.Slots[0].Slot != protocol.EquipmentSlotMainHand {
		t.Fatalf("equipment instance snapshot=%#v", equipmentSnapshot)
	}
	if got := equipmentSnapshot.Slots[0].Item; got.ItemInstanceID != string(instance.ID) || got.ItemArchetypeID != itemID || len(got.Affixes) != 1 || got.Affixes[0].AffixID != string(instance.Affixes[0].ID) {
		t.Fatalf("equipment instance item=%#v", got)
	}
}
