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

func TestJournalCharacterStateOutboxHeadFsyncsBeforeConfirmAndStoreApply(t *testing.T) {
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
	record, ok, err := journalCharacterStateOutboxHead(outbox, journal, store)
	if err != nil || !ok || outbox.Depth() != 0 || journal.LastRecordID() != 1 || record.ExpectedRevision != 0 {
		t.Fatalf("record=%#v ok=%v depth=%d last=%d err=%v", record, ok, outbox.Depth(), journal.LastRecordID(), err)
	}
	if _, exists, err := store.Load(identity); err != nil || exists {
		t.Fatalf("store changed before journal consumer exists=%v err=%v", exists, err)
	}
	if err := applyCharacterStateJournalRecord(store, record); err != nil {
		t.Fatal(err)
	}
	next, err := checkpointStore.Save(journal, record)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint = next
	if checkpoint.RecordID != 1 {
		t.Fatalf("checkpoint=%#v", checkpoint)
	}
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 1 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v exists=%v err=%v intent=%#v", loaded, exists, err, intent)
	}
}

func TestCharacterStateJournalIdenticalSequentialIntentsStillAdvanceRevision(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journal, _ := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	defer journal.Close()
	checkpointStore, _ := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	checkpoint, _ := checkpointStore.Load(journal)
	outbox, _ := characterstate.NewOutbox(4)
	identity := worlddTrustedIdentity(t, "character:alpha")
	snapshot := worlddCharacterSnapshot(900)
	if _, err := outbox.Enqueue(identity, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(identity, snapshot); err != nil {
		t.Fatal(err)
	}
	processed, err := persistCharacterStateOutboxBatch(outbox, journal, checkpointStore, &checkpoint, store)
	if err != nil || processed != 2 || outbox.Depth() != 0 || checkpoint.RecordID != 2 {
		t.Fatalf("processed=%d depth=%d checkpoint=%#v err=%v", processed, outbox.Depth(), checkpoint, err)
	}
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 2 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
	}
	records, err := journal.RecordsAfter(journal.InitialCheckpoint(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ExpectedRevision != 0 || records[1].ExpectedRevision != 1 {
		t.Fatalf("records=%#v", records)
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
	if _, err := journal.Append(characterstate.SaveIntent{IntentID: 99, Identity: identity, Snapshot: snapshot}, 0); err != nil {
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
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 1 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
	}
}

func TestRecoverCharacterStateSaveJournalRecognizesStoreSaveBeforeCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journal, _ := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	defer journal.Close()
	checkpointStore, _ := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	identity := worlddTrustedIdentity(t, "character:idempotent")
	snapshot := worlddCharacterSnapshot(710)
	record, err := journal.Append(characterstate.SaveIntent{IntentID: 7, Identity: identity, Snapshot: snapshot}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyCharacterStateJournalRecord(store, record); err != nil {
		t.Fatal(err)
	}
	before, exists, err := store.Load(identity)
	if err != nil || !exists || before.Revision != 1 {
		t.Fatalf("before=%#v exists=%v err=%v", before, exists, err)
	}
	checkpoint, recovered, err := recoverCharacterStateSaveJournal(journal, checkpointStore, store)
	if err != nil || recovered != 1 || checkpoint.RecordID != 1 {
		t.Fatalf("checkpoint=%#v recovered=%d err=%v", checkpoint, recovered, err)
	}
	after, exists, err := store.Load(identity)
	if err != nil || !exists || after.Revision != 1 || after.Snapshot != snapshot {
		t.Fatalf("after=%#v exists=%v err=%v", after, exists, err)
	}
}

func TestRecoverCharacterStateSaveJournalFailsClosedOnRevisionDivergence(t *testing.T) {
	dir := t.TempDir()
	store, _ := characterstate.Open(filepath.Join(dir, "characters"))
	journal, _ := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal"))
	defer journal.Close()
	checkpointStore, _ := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json"))
	identity := worlddTrustedIdentity(t, "character:diverged")
	journalSnapshot := worlddCharacterSnapshot(800)
	if _, err := journal.Append(characterstate.SaveIntent{IntentID: 1, Identity: identity, Snapshot: journalSnapshot}, 0); err != nil {
		t.Fatal(err)
	}
	other := worlddCharacterSnapshot(500)
	if _, err := store.Save(identity, 0, other); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := checkpointStore.Load(journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := consumeCharacterStateJournalBatch(journal, checkpointStore, &checkpoint, store); !errors.Is(err, characterstate.ErrRevisionConflict) {
		t.Fatalf("err=%v", err)
	}
	if checkpoint.RecordID != 0 {
		t.Fatalf("checkpoint advanced=%#v", checkpoint)
	}
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 1 || loaded.Snapshot != other {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
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
		Context:      respawnpolicy.DeathContextPvP,
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
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 1 {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
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
		HP: hp, MaxHP: 1000, MP: 100, MaxMP: 100,
		Position: world.Position{X: 4, Y: 2, Z: -7, Layer: 1},
		Yaw:      0.75,
	}
}
