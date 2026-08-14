package deathoutcome

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestJournalAppendReopenAndCheckpointRecovery(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "death.journal")
	checkpointPath := filepath.Join(dir, "death.checkpoint.json")

	journal, err := OpenJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	journalID := journal.ID()
	first, err := journal.Append(journalTestEvent(1, 7, 1))
	if err != nil {
		t.Fatal(err)
	}
	second, err := journal.Append(journalTestEvent(2, 8, 1))
	if err != nil {
		t.Fatal(err)
	}
	if first.RecordID != 1 || second.RecordID != 2 || first.EndOffset <= journalHeaderSize() || second.EndOffset <= first.EndOffset {
		t.Fatalf("records first=%#v second=%#v", first, second)
	}

	store, err := NewCheckpointStore(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.Load(journal)
	if err != nil {
		t.Fatal(err)
	}
	if initial.RecordID != 0 || initial.JournalID != journalID || initial.Offset != journalHeaderSize() {
		t.Fatalf("initial=%#v", initial)
	}
	records, err := journal.RecordsAfter(initial, 0)
	if err != nil || len(records) != 2 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	checkpoint, err := store.Save(journal, records[0])
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.RecordID != 1 || checkpoint.Offset != first.EndOffset {
		t.Fatalf("checkpoint=%#v", checkpoint)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.ID() != journalID || reopened.LastRecordID() != 2 || reopened.RepairedTail() {
		t.Fatalf("reopened id=%s last=%d repaired=%v", reopened.ID(), reopened.LastRecordID(), reopened.RepairedTail())
	}
	loaded, err := store.Load(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != checkpoint {
		t.Fatalf("loaded=%#v want=%#v", loaded, checkpoint)
	}
	remaining, err := reopened.RecordsAfter(loaded, 1)
	if err != nil || len(remaining) != 1 || remaining[0].RecordID != 2 || remaining[0].Event != second.Event {
		t.Fatalf("remaining=%#v err=%v", remaining, err)
	}
	final, err := store.Save(reopened, remaining[0])
	if err != nil {
		t.Fatal(err)
	}
	if final.RecordID != 2 || final.Offset != second.EndOffset {
		t.Fatalf("final=%#v", final)
	}
	none, err := reopened.RecordsAfter(final, 64)
	if err != nil || len(none) != 0 {
		t.Fatalf("none=%#v err=%v", none, err)
	}
}

func TestJournalRepairsOnlyIncompleteTrailingFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "death.journal")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Append(journalTestEvent(1, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Declares a 20-byte payload but simulates a crash after writing only two payload bytes.
	if _, err := file.Write([]byte{0, 0, 0, 20, '{', 'x'}); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.RepairedTail() || reopened.LastRecordID() != 1 {
		t.Fatalf("repaired=%v last=%d", reopened.RepairedTail(), reopened.LastRecordID())
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != record.EndOffset {
		t.Fatalf("size=%d want=%d", stat.Size(), record.EndOffset)
	}
}

func TestJournalRejectsCRCOrRecordCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "death.journal")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(journalTestEvent(1, 10, 1)); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	offset := journalHeaderSize() + 4 + 8
	one := []byte{0}
	if _, err := file.ReadAt(one, offset); err != nil {
		t.Fatal(err)
	}
	one[0] ^= 0xff
	if _, err := file.WriteAt(one, offset); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenJournal(path); !errors.Is(err, ErrCorruptJournal) {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckpointRejectsDifferentJournalAheadAndWrongOffset(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.journal")
	secondPath := filepath.Join(dir, "second.journal")
	checkpointPath := filepath.Join(dir, "checkpoint.json")

	first, err := OpenJournal(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	record, err := first.Append(journalTestEvent(1, 11, 1))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewCheckpointStore(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(first, record); err != nil {
		t.Fatal(err)
	}

	second, err := OpenJournal(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := store.Load(second); !errors.Is(err, ErrCheckpointJournalMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}

	ahead := checkpointWire{SchemaVersion: checkpointSchemaVersion, JournalID: first.ID(), RecordID: 2, Offset: record.EndOffset}
	writeCheckpointWire(t, checkpointPath, ahead)
	if _, err := store.Load(first); !errors.Is(err, ErrCheckpointAhead) {
		t.Fatalf("ahead err=%v", err)
	}

	wrongOffset := checkpointWire{SchemaVersion: checkpointSchemaVersion, JournalID: first.ID(), RecordID: 1, Offset: record.EndOffset + 1}
	writeCheckpointWire(t, checkpointPath, wrongOffset)
	if _, err := store.Load(first); !errors.Is(err, ErrCheckpointOffsetMismatch) {
		t.Fatalf("offset err=%v", err)
	}
}

func TestJournalAppendAfterCloseDoesNotAdvanceTruth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "death.journal")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(journalTestEvent(1, 12, 1)); !errors.Is(err, ErrJournalClosed) {
		t.Fatalf("err=%v", err)
	}
}

func journalTestEvent(eventID, entityID, defeatRevision uint64) Event {
	defeatedTick := uint64(100 + defeatRevision)
	return Event{
		EventID:                    eventID,
		EntityID:                   world.EntityID(entityID),
		DefeatRevision:             defeatRevision,
		Context:                    respawnpolicy.DeathContextPvE,
		DefeatedTick:               defeatedTick,
		RespawnPolicyRevision:      "respawn-test",
		DeathPenaltyPolicyRevision: "penalty-test",
		Respawn: RespawnBinding{
			Scheduled:    true,
			SpawnPointID: "checkpoint",
			SpawnClass:   respawnpolicy.SpawnClassCheckpoint,
			Position:     world.Position{X: 10, Y: 2, Z: -3, Layer: 4},
			DueTick:      defeatedTick + 20,
		},
		PenaltyTransactionApplied: true,
		CheckpointForfeited:       true,
	}
}

func writeCheckpointWire(t *testing.T, path string, wire checkpointWire) {
	t.Helper()
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
