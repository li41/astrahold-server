package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
)

func TestStoreV11RoundTripsPrimaryStats(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v11")
	snapshot := testSnapshot()
	snapshot.PrimaryStats = characterstats.Primary{Strength: 17, Agility: 23}

	saved, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if saved.SchemaVersion != PrimaryStatsSchemaVersion {
		t.Fatalf("schema=%d want=%d", saved.SchemaVersion, PrimaryStatsSchemaVersion)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.PrimaryStats != snapshot.PrimaryStats {
		t.Fatalf("primary stats changed: got=%#v want=%#v", loaded.Snapshot.PrimaryStats, snapshot.PrimaryStats)
	}
}

func TestStoreV10RejectsPrimaryStatsField(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v10-reject")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: ItemInstanceSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: &wirePrimaryStats{Strength: 1, Agility: 2},
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("v10 primary stats load err=%v", err)
	}
}

func TestStoreV10MigratesToZeroPrimaryStats(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v10-zero")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: ItemInstanceSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.PrimaryStats != (characterstats.Primary{}) {
		t.Fatalf("legacy primary stats=%#v", loaded.Snapshot.PrimaryStats)
	}
}

func TestSaveJournalV10RoundTripsPrimaryStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "primary-stats.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.PrimaryStats = characterstats.Primary{Strength: 29, Agility: 31}
	intent := SaveIntent{IntentID: 81, Identity: trusted(t, "character:primary-stats-journal"), Snapshot: snapshot}
	if _, err := journal.Append(intent, 0); err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].Intent.Snapshot.PrimaryStats != snapshot.PrimaryStats {
		t.Fatalf("journal primary stats=%#v", records)
	}
}

func TestSaveJournalV9RejectsPrimaryStatsField(t *testing.T) {
	snapshot := testSnapshot()
	identity := trusted(t, "character:primary-stats-journal-v9")
	wire := saveJournalWireRecord{
		SchemaVersion: ItemInstanceSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 82, CharacterID: string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
			Inventory: snapshot.Inventory,
			PrimaryStats: &wirePrimaryStats{Strength: 1, Agility: 1},
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
		t.Fatalf("v9 primary stats journal err=%v", err)
	}
}