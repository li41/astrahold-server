package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func durableTestInstances(t *testing.T) (InventoryState, iteminstance.Instance, iteminstance.Instance) {
	t.Helper()
	bag := iteminstance.Instance{
		ID:              "item-instance:bag-1",
		ItemArchetypeID: "item_mid_test_sword",
		Affixes: []equipmentaffix.Affix{{
			ID: equipmentaffix.AffixPhysicalDamage, Strength: 2, Value: 2,
		}},
	}
	main := iteminstance.Instance{
		ID:              "item-instance:main-1",
		ItemArchetypeID: "item_high_test_sword",
		Affixes: []equipmentaffix.Affix{
			{ID: equipmentaffix.AffixCriticalRating, Strength: 3, Value: 3},
			{ID: equipmentaffix.AffixStrength, Strength: 1, Value: 1},
		},
	}
	state, err := NewInventoryStateWithInstances(nil, []iteminstance.Instance{bag}, "", "", &main, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state, bag, main
}

func TestStoreV10RoundTripsUniqueItemInstancesWithoutReroll(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:item-instance-v10")
	inventoryState, _, _ := durableTestInstances(t)
	snapshot := testSnapshot()
	snapshot.Inventory = inventoryState

	saved, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if saved.SchemaVersion != ItemInstanceSchemaVersion {
		t.Fatalf("schema=%d want=%d", saved.SchemaVersion, ItemInstanceSchemaVersion)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.Inventory != inventoryState {
		t.Fatalf("inventory changed across store round trip: got=%#v want=%#v", loaded.Snapshot.Inventory, inventoryState)
	}
}

func TestStoreV9RejectsItemInstanceFields(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:item-instance-v9-reject")
	inventoryState, _, _ := durableTestInstances(t)
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: ClasslessSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: inventoryState,
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("v9 item instance load err=%v", err)
	}
}

func TestSaveJournalV9RoundTripsUniqueItemInstancesWithoutReroll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item-instances.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	inventoryState, _, _ := durableTestInstances(t)
	snapshot := testSnapshot()
	snapshot.Inventory = inventoryState
	intent := SaveIntent{IntentID: 71, Identity: trusted(t, "character:item-instance-journal"), Snapshot: snapshot}
	if _, err := journal.Append(intent, 0); err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].Intent.Snapshot.Inventory != inventoryState {
		t.Fatalf("journal changed item instances: %#v", records)
	}
}

func TestSaveJournalV8RejectsItemInstanceFields(t *testing.T) {
	inventoryState, _, _ := durableTestInstances(t)
	snapshot := testSnapshot()
	identity := trusted(t, "character:item-instance-journal-v8")
	wire := saveJournalWireRecord{
		SchemaVersion: ClasslessSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 72, CharacterID: string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
			Inventory: inventoryState,
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
		t.Fatalf("v8 item instance journal err=%v", err)
	}
}
