package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/appearance"
)

func TestStoreCurrentRoundTripsSelectedSkin(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:skin-current")
	snapshot := testSnapshot()
	snapshot.SkinID = appearance.PeasantGirl

	saved, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if saved.SchemaVersion != AppearanceSchemaVersion {
		t.Fatalf("schema=%d want=%d", saved.SchemaVersion, AppearanceSchemaVersion)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.SkinID != appearance.PeasantGirl {
		t.Fatalf("skin=%q want=%q", loaded.Snapshot.SkinID, appearance.PeasantGirl)
	}
}

func TestStoreV12MigratesToNoSelectedSkin(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:skin-v12-migrate")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: SixPrimaryStatsSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats),
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.SkinID != appearance.None {
		t.Fatalf("legacy skin=%q want empty", loaded.Snapshot.SkinID)
	}
}

func TestStoreV12RejectsSkinSmuggling(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:skin-v12-smuggle")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: SixPrimaryStatsSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		SkinID: string(appearance.PeasantGirl),
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats),
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("v12 skin smuggling err=%v", err)
	}
}

func TestSaveJournalCurrentRoundTripsSelectedSkin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skin.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.SkinID = appearance.KachujinGRosales
	intent := SaveIntent{IntentID: 91, Identity: trusted(t, "character:skin-journal"), Snapshot: snapshot}
	if _, err := journal.Append(intent, 0); err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].Intent.Snapshot.SkinID != appearance.KachujinGRosales {
		t.Fatalf("journal records=%#v", records)
	}
}

func TestSaveJournalV11RejectsSkinSmuggling(t *testing.T) {
	snapshot := testSnapshot()
	identity := trusted(t, "character:skin-journal-v11-smuggle")
	wire := saveJournalWireRecord{
		SchemaVersion: SixPrimaryStatsSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 92, CharacterID: string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			SkinID: string(appearance.PeasantGirl),
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
			Inventory: snapshot.Inventory,
			PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats),
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
		t.Fatalf("v11 skin smuggling err=%v", err)
	}
}

func TestCurrentPersistenceRejectsUnknownSkin(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.SkinID = appearance.SkinID("skin_unknown")
	if _, err := store.Save(trusted(t, "character:skin-invalid"), 0, snapshot); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("store unknown skin err=%v", err)
	}
}
