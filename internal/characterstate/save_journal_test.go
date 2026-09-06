package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSaveJournalAppendReopenRoundTripsAliveAndDefeated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "character-state-saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	journalID := journal.ID()
	alive := SaveIntent{IntentID: 11, Identity: trusted(t, "character:journal-alive"), Snapshot: testSnapshot()}
	defeatedSnapshot := testSnapshot()
	defeatedSnapshot.HP = 0
	defeatedSnapshot.Defeated = true
	defeatedSnapshot.Respawn = DefeatedRespawn{
		Context: respawnpolicy.DeathContextSiege,
		SpawnPointID: "siege-staging", SpawnClass: respawnpolicy.SpawnClassSiege,
		Position: world.Position{X: 21, Y: 0, Z: -8, Layer: 2}, RemainingTicks: 37,
		CheckpointID: "checkpoint-west",
	}
	defeated := SaveIntent{IntentID: 12, Identity: trusted(t, "character:journal-defeated"), Snapshot: defeatedSnapshot}
	first, err := journal.Append(alive, 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := journal.Append(defeated, 8)
	if err != nil {
		t.Fatal(err)
	}
	if first.RecordID != 1 || first.ExpectedRevision != 3 || second.RecordID != 2 || second.ExpectedRevision != 8 || second.EndOffset <= first.EndOffset {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.ID() != journalID || reopened.LastRecordID() != 2 {
		t.Fatalf("id=%s last=%d", reopened.ID(), reopened.LastRecordID())
	}
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Intent != alive || records[0].ExpectedRevision != 3 || records[1].Intent != defeated || records[1].ExpectedRevision != 8 {
		t.Fatalf("records=%#v", records)
	}
}

func TestSaveJournalDecodesLegacyV1WithDefaultMP(t *testing.T) {
	snapshot := testSnapshot()
	identity := trusted(t, "character:journal-v1")
	wire := saveJournalWireRecord{
		SchemaVersion:    LegacySaveJournalSchemaVersion,
		RecordID:         1,
		ExpectedRevision: 0,
		IntentID:         7,
		CharacterID:      string(identity.ID),
		Snapshot: saveJournalWireSnapshot{
			WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
			HP: snapshot.HP, MaxHP: snapshot.MaxHP, Defeated: false,
			X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		},
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	_, _, intent, err := decodeSaveJournalRecord(payload)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Snapshot.MP != LegacyDefaultMaxMP || intent.Snapshot.MaxMP != LegacyDefaultMaxMP {
		t.Fatalf("legacy journal mp=%d/%d; want %d/%d", intent.Snapshot.MP, intent.Snapshot.MaxMP, LegacyDefaultMaxMP, LegacyDefaultMaxMP)
	}
}

func TestSaveJournalCheckpointReopenResumesAfterDurableRecord(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "saves.journal")
	checkpointPath := filepath.Join(dir, "saves.checkpoint.json")
	journal, err := OpenSaveJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := journal.Append(SaveIntent{IntentID: 1, Identity: trusted(t, "character:first"), Snapshot: testSnapshot()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot := testSnapshot()
	secondSnapshot.HP = 700
	second, err := journal.Append(SaveIntent{IntentID: 2, Identity: trusted(t, "character:second"), Snapshot: secondSnapshot}, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpointStore, err := NewSaveCheckpointStore(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := checkpointStore.Save(journal, first)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.RecordID != 1 {
		t.Fatalf("checkpoint=%#v", checkpoint)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSaveJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStore, err := NewSaveCheckpointStore(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopenedStore.Load(reopened)
	if err != nil {
		t.Fatal(err)
	}
	records, err := reopened.RecordsAfter(loaded, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RecordID != second.RecordID || records[0].Intent != second.Intent || records[0].ExpectedRevision != 0 {
		t.Fatalf("records=%#v", records)
	}
}

func TestSaveJournalRepairsOnlyIncompleteTrailingFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(SaveIntent{IntentID: 1, Identity: trusted(t, "character:torn"), Snapshot: testSnapshot()}, 0); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0, 0}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.RepairedTail() || reopened.LastRecordID() != 1 {
		t.Fatalf("repaired=%v last=%d", reopened.RepairedTail(), reopened.LastRecordID())
	}
}

func TestSaveJournalCRCcorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(SaveIntent{IntentID: 1, Identity: trusted(t, "character:crc"), Snapshot: testSnapshot()}, 0); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	offset := saveJournalHeaderSize() + 4 + 8
	var b [1]byte
	if _, err := file.ReadAt(b[:], offset); err != nil {
		file.Close()
		t.Fatal(err)
	}
	b[0] ^= 0xff
	if _, err := file.WriteAt(b[:], offset); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSaveJournal(path); !errors.Is(err, ErrCorruptSaveJournal) {
		t.Fatalf("err=%v", err)
	}
}
