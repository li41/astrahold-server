package characterstate

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
)

func TestSaveJournalV5RoundTripsCanonicalClassID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "class-saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot()
	snapshot.ClassID = classid.Breaker
	intent := SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-class"), Snapshot: snapshot}
	record, err := journal.Append(intent, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RecordID != record.RecordID || records[0].Intent.Snapshot.ClassID != classid.Breaker {
		t.Fatalf("records=%#v", records)
	}
}

func TestSaveJournalV4MigratesToUnassignedClassID(t *testing.T) {
	snapshot := testSnapshot()
	wire := saveJournalWireRecord{
		SchemaVersion: EquipmentSaveJournalSchemaVersion,
		RecordID: 1,
		ExpectedRevision: 0,
		IntentID: 1,
		CharacterID: string(trusted(t, "character:journal-v4-class").ID),
		Snapshot: snapshotToSaveJournalWire(snapshot),
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	_, _, intent, err := decodeSaveJournalRecord(payload)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Snapshot.ClassID != "" {
		t.Fatalf("legacy ClassID=%q want unassigned", intent.Snapshot.ClassID)
	}
}

func TestSaveJournalRejectsClassIDThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  uint16
		classID string
	}{
		{name: "legacy schema carries class", schema: EquipmentSaveJournalSchemaVersion, classID: string(classid.Oathguard)},
		{name: "current schema unknown class", schema: ClassSaveJournalSchemaVersion, classID: "class_guard"},
		{name: "current schema whitespace class", schema: ClassSaveJournalSchemaVersion, classID: "class_oathguard "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testSnapshot()
			wireSnapshot := snapshotToSaveJournalWire(snapshot)
			wireSnapshot.ClassID = tc.classID
			wire := saveJournalWireRecord{
				SchemaVersion: tc.schema,
				RecordID: 1,
				ExpectedRevision: 0,
				IntentID: 1,
				CharacterID: string(trusted(t, "character:journal-class-corrupt").ID),
				Snapshot: wireSnapshot,
			}
			payload, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
				t.Fatalf("decode err=%v", err)
			}
		})
	}
}

func TestSaveJournalRejectsInvalidClassIDBeforeAppend(t *testing.T) {
	journal, err := OpenSaveJournal(filepath.Join(t.TempDir(), "invalid-class.journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	snapshot := testSnapshot()
	snapshot.ClassID = classid.ID("class_oathguard ")
	_, err = journal.Append(SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-invalid-class"), Snapshot: snapshot}, 0)
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("append err=%v", err)
	}
}
