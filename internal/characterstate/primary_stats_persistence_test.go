package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
)

func TestStoreV12RoundTripsSixPrimaryStats(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v12")
	snapshot := testSnapshot()
	snapshot.PrimaryStats = characterstats.Primary{Strength: 17, Agility: 18, Constitution: 19, Intelligence: 20, Spirit: 21, Charisma: 22}

	saved, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if saved.SchemaVersion != SixPrimaryStatsSchemaVersion {
		t.Fatalf("schema=%d want=%d", saved.SchemaVersion, SixPrimaryStatsSchemaVersion)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.PrimaryStats != snapshot.PrimaryStats {
		t.Fatalf("primary stats changed: got=%#v want=%#v", loaded.Snapshot.PrimaryStats, snapshot.PrimaryStats)
	}
}

func TestStoreV11MigratesTwoStatsIntoSix(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v11-migrate")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: PrimaryStatsSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: &wirePrimaryStats{Strength: 17, Agility: 18},
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	want := characterstats.Primary{Strength: 17, Agility: 18, Constitution: 10, Intelligence: 10, Spirit: 10, Charisma: 10}
	if loaded.Snapshot.PrimaryStats != want { t.Fatalf("got=%#v want=%#v", loaded.Snapshot.PrimaryStats, want) }
}

func TestStoreV11ZeroInterimStatsMigrateToNeutralBaseline(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v11-zero")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: PrimaryStatsSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: &wirePrimaryStats{},
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.PrimaryStats != characterstats.DefaultPrimary() { t.Fatalf("got=%#v", loaded.Snapshot.PrimaryStats) }
}

func TestStoreV11RejectsBelowBaselineInterimStats(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v11-invalid")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: PrimaryStatsSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
		PrimaryStats: &wirePrimaryStats{Strength: 9, Agility: 10},
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) { t.Fatalf("err=%v", err) }
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
		PrimaryStats: &wirePrimaryStats{Strength: 10, Agility: 10},
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) { t.Fatalf("v10 primary stats load err=%v", err) }
}

func TestStoreV10MigratesToNeutralPrimaryStats(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:primary-stats-v10-neutral")
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
	if loaded.Snapshot.PrimaryStats != characterstats.DefaultPrimary() { t.Fatalf("legacy primary stats=%#v", loaded.Snapshot.PrimaryStats) }
}

func TestSaveJournalV11RoundTripsSixPrimaryStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "primary-stats.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.PrimaryStats = characterstats.Primary{Strength: 29, Agility: 31, Constitution: 18, Intelligence: 24, Spirit: 20, Charisma: 15}
	intent := SaveIntent{IntentID: 81, Identity: trusted(t, "character:primary-stats-journal"), Snapshot: snapshot}
	if _, err := journal.Append(intent, 0); err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].Intent.Snapshot.PrimaryStats != snapshot.PrimaryStats { t.Fatalf("journal primary stats=%#v", records) }
}

func TestSaveJournalV10MigratesTwoStatsIntoSix(t *testing.T) {
	snapshot := testSnapshot()
	identity := trusted(t, "character:primary-stats-journal-v10")
	wire := saveJournalWireRecord{
		SchemaVersion: PrimaryStatsSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 82, CharacterID: string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
			Inventory: snapshot.Inventory,
			PrimaryStats: &wirePrimaryStats{Strength: 16, Agility: 19},
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	_, _, intent, err := decodeSaveJournalRecord(payload)
	if err != nil { t.Fatal(err) }
	want := characterstats.Primary{Strength: 16, Agility: 19, Constitution: 10, Intelligence: 10, Spirit: 10, Charisma: 10}
	if intent.Snapshot.PrimaryStats != want { t.Fatalf("got=%#v want=%#v", intent.Snapshot.PrimaryStats, want) }
}

func TestSaveJournalV9RejectsPrimaryStatsField(t *testing.T) {
	snapshot := testSnapshot()
	identity := trusted(t, "character:primary-stats-journal-v9")
	wire := saveJournalWireRecord{
		SchemaVersion: ItemInstanceSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 83, CharacterID: string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
			Inventory: snapshot.Inventory,
			PrimaryStats: &wirePrimaryStats{Strength: 10, Agility: 10},
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) { t.Fatalf("v9 primary stats journal err=%v", err) }
}