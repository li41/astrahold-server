package characterstate

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStoreV4InventoryRoundTripPreservesStacksAndMainHand(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:inventory-v4")
	inventoryState, err := NewInventoryState([]InventoryStack{
		{ItemArchetypeID: "item_minor_mana_potion", Quantity: 2},
		{ItemArchetypeID: "item_minor_healing_potion", Quantity: 3},
	}, "item_training_blade")
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.Inventory = inventoryState
	saved, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded != saved { t.Fatalf("loaded=%#v saved=%#v", loaded, saved) }
	if !loaded.Snapshot.Inventory.Initialized || loaded.Snapshot.Inventory.MainHand != "item_training_blade" {
		t.Fatalf("inventory=%#v", loaded.Snapshot.Inventory)
	}
	stacks, err := loaded.Snapshot.Inventory.Stacks()
	if err != nil { t.Fatal(err) }
	if len(stacks) != 2 || stacks[0].ItemArchetypeID != "item_minor_healing_potion" || stacks[1].ItemArchetypeID != "item_minor_mana_potion" {
		t.Fatalf("stacks=%#v", stacks)
	}
}

func TestStoreV3InventoryMigrationRemainsUninitialized(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:inventory-v3")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: ResourceSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.Inventory != (InventoryState{}) || loaded.Snapshot.Inventory.Initialized {
		t.Fatalf("legacy inventory=%#v", loaded.Snapshot.Inventory)
	}
}

func TestSaveJournalV3InventoryRoundTripAndV2Migration(t *testing.T) {
	identity := trusted(t, "character:journal-inventory")
	snapshot := testSnapshot()
	inventoryState, err := NewInventoryState([]InventoryStack{{ItemArchetypeID: "item_minor_healing_potion", Quantity: 4}}, "item_training_blade")
	if err != nil { t.Fatal(err) }
	snapshot.Inventory = inventoryState
	intent := SaveIntent{IntentID: 7, Identity: identity, Snapshot: snapshot}
	payload, err := encodeSaveJournalRecord(3, 2, intent)
	if err != nil { t.Fatal(err) }
	_, _, decoded, err := decodeSaveJournalRecord(payload)
	if err != nil { t.Fatal(err) }
	if decoded != intent { t.Fatalf("decoded=%#v intent=%#v", decoded, intent) }

	legacyWire := saveJournalWireRecord{
		SchemaVersion: ResourceSaveJournalSchemaVersion,
		RecordID: 4, ExpectedRevision: 3, IntentID: 8, CharacterID: string(identity.ID),
		Snapshot: snapshotToSaveJournalWire(testSnapshot()),
	}
	legacyWire.Snapshot.Inventory = InventoryState{}
	legacyPayload, err := json.Marshal(legacyWire)
	if err != nil { t.Fatal(err) }
	_, _, legacyDecoded, err := decodeSaveJournalRecord(legacyPayload)
	if err != nil { t.Fatal(err) }
	if legacyDecoded.Snapshot.Inventory != (InventoryState{}) || legacyDecoded.Snapshot.Inventory.Initialized {
		t.Fatalf("legacy decoded inventory=%#v", legacyDecoded.Snapshot.Inventory)
	}
}
