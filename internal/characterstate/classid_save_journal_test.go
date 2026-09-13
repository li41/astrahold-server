package characterstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
)

func TestSaveJournalV8SnapshotIsClassless(t *testing.T) {
	path := filepath.Join(t.TempDir(), "classless-saves.journal")
	journal, err := OpenSaveJournal(path); if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	intent := SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-classless"), Snapshot: snapshot}
	record, err := journal.Append(intent, 0); if err != nil { t.Fatal(err) }
	if record.Intent.Snapshot != snapshot { t.Fatalf("append snapshot=%#v want=%#v", record.Intent.Snapshot, snapshot) }
	if err := journal.Close(); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
	if bytes.Contains(data, []byte("class_id")) { t.Fatalf("v8 journal still contains class_id") }
	reopened, err := OpenSaveJournal(path); if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0); if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].RecordID != record.RecordID || records[0].Intent.Snapshot != snapshot { t.Fatalf("records=%#v", records) }
}

func TestSaveJournalV7ValidatesThenDiscardsLegacyClassID(t *testing.T) {
	snapshot := testSnapshot()
	wireSnapshot := snapshotToSaveJournalWire(snapshot); wireSnapshot.ClassID = string(classid.Oathguard)
	wire := saveJournalWireRecord{SchemaVersion: LearnedSkillsSaveJournalSchemaVersion, RecordID: 1, ExpectedRevision: 0, IntentID: 1, CharacterID: string(trusted(t, "character:journal-v7-class").ID), Snapshot: wireSnapshot}
	payload, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
	_, _, intent, err := decodeSaveJournalRecord(payload); if err != nil { t.Fatal(err) }
	if intent.Snapshot != snapshot { t.Fatalf("legacy snapshot=%#v want=%#v", intent.Snapshot, snapshot) }
}

func TestSaveJournalRejectsClassIDThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct{name string; schema uint16; classID string}{
		{name: "pre-class schema carries class", schema: EquipmentSaveJournalSchemaVersion, classID: string(classid.Oathguard)},
		{name: "legacy class schema unknown class", schema: ClassSaveJournalSchemaVersion, classID: "class_guard"},
		{name: "legacy class schema whitespace class", schema: LearnedSkillsSaveJournalSchemaVersion, classID: "class_oathguard "},
		{name: "classless schema smuggles retired class", schema: ClasslessSaveJournalSchemaVersion, classID: string(classid.Oathguard)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testSnapshot(); wireSnapshot := snapshotToSaveJournalWire(snapshot); wireSnapshot.ClassID = tc.classID
			wire := saveJournalWireRecord{SchemaVersion: tc.schema, RecordID: 1, ExpectedRevision: 0, IntentID: 1, CharacterID: string(trusted(t, "character:journal-class-corrupt").ID), Snapshot: wireSnapshot}
			payload, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
			if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) { t.Fatalf("decode err=%v", err) }
		})
	}
}
