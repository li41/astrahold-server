package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const worlddCharacterStateSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var worlddCharacterWorld = protocol.WorldIdentity{
	WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: worlddCharacterStateSHA,
}

func TestIngestCharacterStateOutboxBatchJournalsBeforeConfirm(t *testing.T) {
	dir := t.TempDir()
	store, err := characterstate.Open(filepath.Join(dir, "characters"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	checkpointStore, err := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := checkpointStore.Load(journal)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	identity := worlddTrustedIdentity(t, "character:alpha")
	snapshot := worlddCharacterSnapshot(900)
	intent, err := outbox.Enqueue(identity, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	processed, err := ingestCharacterStateOutboxBatch(outbox, journal)
	if err != nil || processed != 1 || outbox.Depth() != 0 || journal.LastRecordID() != 1 {
		t.Fatalf("processed=%d depth=%d last=%d err=%v", processed, outbox.Depth(), journal.LastRecordID(), err)
	}
	if _, ok, err := store.Load(identity); err != nil || ok {
		t.Fatalf("store changed before journal consumer ok=%v err=%v", ok, err)
	}
	consumed, err := consumeCharacterStateJournalBatch(journal, checkpointStore, &checkpoint, store)
	if err != nil || consumed != 1 || checkpoint.RecordID != 1 {
		t.Fatalf("consumed=%d checkpoint=%#v err=%v", consumed, checkpoint, err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v ok=%v err=%v intent=%#v", loaded, ok, err, intent)
	}
}

func TestCharacterStateJournalSequentialIntentsAdvanceRevision(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journal, _ := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	defer journal.Close()
	checkpointStore, _ := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	checkpoint, _ := checkpointStore.Load(journal)
	outbox, _ := characterstate.NewOutbox(4)
	identity := worlddTrustedIdentity(t, "character:alpha")
	first := worlddCharacterSnapshot(900)
	second := worlddCharacterSnapshot(700)
	second.Position = world.Position{X: 20, Y: 1, Z: -8, Layer: 3}
	if _, err := outbox.Enqueue(identity, first); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(identity, second); err != nil {
		t.Fatal(err)
	}
	if processed, err := ingestCharacterStateOutboxBatch(outbox, journal); err != nil || processed != 2 {
		t.Fatalf("ingest=%d err=%v", processed, err)
	}
	if processed, err := consumeCharacterStateJournalBatch(journal, checkpointStore, &checkpoint, store); err != nil || processed != 2 {
		t.Fatalf("consume=%d err=%v", processed, err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 2 || loaded.Snapshot != second {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
}

func TestRecoverCharacterStateSaveJournalReplaysDurableUncheckpointedIntent(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journalPath := filepath.Join(dir, "saves.journal")
	checkpointPath := filepath.Join(dir, "saves.checkpoint.json")
	journal, err := characterstate.OpenSaveJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	identity := worlddTrustedIdentity(t, "character:restart")
	snapshot := worlddCharacterSnapshot(620)
	if _, err := journal.Append(characterstate.SaveIntent{IntentID: 99, Identity: identity, Snapshot: snapshot}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := characterstate.OpenSaveJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	checkpointStore, err := characterstate.NewSaveCheckpointStore(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, recovered, err := recoverCharacterStateSaveJournal(reopened, checkpointStore, store)
	if err != nil || recovered != 1 || checkpoint.RecordID != 1 {
		t.Fatalf("checkpoint=%#v recovered=%d err=%v", checkpoint, recovered, err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
}

func TestRecoverCharacterStateSaveJournalIsIdempotentAfterStoreSaveBeforeCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journal, _ := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	defer journal.Close()
	checkpointStore, _ := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	identity := worlddTrustedIdentity(t, "character:idempotent")
	snapshot := worlddCharacterSnapshot(710)
	record, err := journal.Append(characterstate.SaveIntent{IntentID: 7, Identity: identity, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyCharacterStateJournalRecord(store, record); err != nil {
		t.Fatal(err)
	}
	before, ok, err := store.Load(identity)
	if err != nil || !ok || before.Revision != 1 {
		t.Fatalf("before=%#v ok=%v err=%v", before, ok, err)
	}
	checkpoint, recovered, err := recoverCharacterStateSaveJournal(journal, checkpointStore, store)
	if err != nil || recovered != 1 || checkpoint.RecordID != 1 {
		t.Fatalf("checkpoint=%#v recovered=%d err=%v", checkpoint, recovered, err)
	}
	after, ok, err := store.Load(identity)
	if err != nil || !ok || after.Revision != 1 || after.Snapshot != snapshot {
		t.Fatalf("after=%#v ok=%v err=%v", after, ok, err)
	}
}

func TestLoadRestoreFlushesPendingLeaveThroughJournalBeforeRead(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	outbox, _ := characterstate.NewOutbox(4)
	identity := worlddTrustedIdentity(t, "character:reconnect")
	first := worlddCharacterSnapshot(900)
	if _, err := store.Save(identity, 0, first); err != nil {
		t.Fatal(err)
	}
	latest := worlddCharacterSnapshot(650)
	latest.Position = world.Position{X: 33, Z: -11, Layer: 2}
	if _, err := outbox.Enqueue(identity, latest); err != nil {
		t.Fatal(err)
	}
	persistence := newWorlddCharacterPersistence(t, dir, outbox, store)
	defer persistence.journal.Close()
	restore, ok, err := persistence.LoadRestore(identity)
	if err != nil || !ok {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
	}
	if outbox.Depth() != 0 || persistence.checkpoint.RecordID != 1 || restore.Revision != 2 || restore.HP != 650 || restore.Transform.Position != latest.Position {
		t.Fatalf("restore=%#v depth=%d checkpoint=%#v", restore, outbox.Depth(), persistence.checkpoint)
	}
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 2 || loaded.Snapshot != latest {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
	}
}

func TestLoadRestoreRejectsWorldMismatch(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	outbox, _ := characterstate.NewOutbox(2)
	identity := worlddTrustedIdentity(t, "character:world-mismatch")
	snapshot := worlddCharacterSnapshot(800)
	snapshot.World.Revision = "old-world"
	if _, err := store.Save(identity, 0, snapshot); err != nil {
		t.Fatal(err)
	}
	persistence := newWorlddCharacterPersistence(t, dir, outbox, store)
	defer persistence.journal.Close()
	if _, ok, err := persistence.LoadRestore(identity); ok || !errors.Is(err, worldruntime.ErrCharacterRestoreWorldMismatch) {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestLoadRestoreAcceptsCompleteDefeatedV2Record(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	outbox, _ := characterstate.NewOutbox(2)
	identity := worlddTrustedIdentity(t, "character:defeated")
	snapshot := worlddCharacterSnapshot(0)
	snapshot.Defeated = true
	snapshot.Respawn = characterstate.DefeatedRespawn{
		Context: respawnpolicy.DeathContextPvP,
		SpawnPointID: "safe", SpawnClass: respawnpolicy.SpawnClassSafe,
		Position: world.Position{X: 12, Z: -6, Layer: 1}, RemainingTicks: 23,
		CheckpointID: "checkpoint",
	}
	if _, err := store.Save(identity, 0, snapshot); err != nil {
		t.Fatal(err)
	}
	persistence := newWorlddCharacterPersistence(t, dir, outbox, store)
	defer persistence.journal.Close()
	restore, ok, err := persistence.LoadRestore(identity)
	if err != nil || !ok {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
	}
	if !restore.Defeated || restore.SchemaVersion != characterstate.SchemaVersion || restore.Respawn != snapshot.Respawn {
		t.Fatalf("restore=%#v", restore)
	}
}

func TestLoadRestoreMissingRecordUsesFreshBootstrap(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	outbox, _ := characterstate.NewOutbox(2)
	persistence := newWorlddCharacterPersistence(t, dir, outbox, store)
	defer persistence.journal.Close()
	identity := worlddTrustedIdentity(t, "character:new")
	if restore, ok, err := persistence.LoadRestore(identity); err != nil || ok || restore != (worldruntime.CharacterRestore{}) {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
	}
}

func TestRunCharacterStateStoreShutdownDrainsOutboxAndJournal(t *testing.T) {
	dir := t.TempDir()
	store, err := characterstate.Open(filepath.Join(dir, "characters"))
	if err != nil {
		t.Fatal(err)
	}
	outbox, _ := characterstate.NewOutbox(2)
	identity := worlddTrustedIdentity(t, "character:shutdown")
	if _, err := outbox.Enqueue(identity, worlddCharacterSnapshot(800)); err != nil {
		t.Fatal(err)
	}
	persistence := newWorlddCharacterPersistence(t, dir, outbox, store)
	defer persistence.journal.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runCharacterStateStore(ctx, persistence); err != nil {
		t.Fatal(err)
	}
	if outbox.Depth() != 0 || persistence.checkpoint.RecordID != persistence.journal.LastRecordID() {
		t.Fatalf("depth=%d checkpoint=%#v last=%d", outbox.Depth(), persistence.checkpoint, persistence.journal.LastRecordID())
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
}

func newWorlddCharacterPersistence(t *testing.T, dir string, outbox *characterstate.Outbox, store *characterstate.Store) *characterStatePersistence {
	t.Helper()
	journal, err := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	if err != nil {
		t.Fatal(err)
	}
	checkpointStore, err := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	if err != nil {
		journal.Close()
		t.Fatal(err)
	}
	checkpoint, err := checkpointStore.Load(journal)
	if err != nil {
		journal.Close()
		t.Fatal(err)
	}
	return newCharacterStatePersistence(outbox, journal, checkpointStore, checkpoint, store, worlddCharacterWorld)
}

func worlddTrustedIdentity(t *testing.T, id string) characteridentity.Binding {
	t.Helper()
	binding, err := characteridentity.NewTrusted(id)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func worlddCharacterSnapshot(hp uint32) characterstate.Snapshot {
	return characterstate.Snapshot{
		World: characterstate.WorldRef{
			WorldID: worlddCharacterWorld.WorldID, Revision: worlddCharacterWorld.Revision, GameplaySHA256: worlddCharacterWorld.GameplaySHA256,
		},
		HP: hp, MaxHP: 1000,
		Position: world.Position{X: 4, Y: 2, Z: -7, Layer: 1},
		Yaw:      0.75,
	}
}
