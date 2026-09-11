package main

import (
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classid"
)

func TestPersistCharacterStateOutboxKeepsClassOnlyInProcessLocalCompletion(t *testing.T) {
	dir := t.TempDir()
	store, err := characterstate.Open(filepath.Join(dir, "characters")); if err != nil { t.Fatal(err) }
	journal, err := characterstate.OpenSaveJournal(filepath.Join(dir, "saves.journal")); if err != nil { t.Fatal(err) }
	defer journal.Close()
	checkpointStore, err := characterstate.NewSaveCheckpointStore(filepath.Join(dir, "saves.checkpoint.json")); if err != nil { t.Fatal(err) }
	checkpoint, err := checkpointStore.Load(journal); if err != nil { t.Fatal(err) }
	outbox, err := characterstate.NewOutbox(4); if err != nil { t.Fatal(err) }
	identity := worlddTrustedIdentity(t, "character:class-completion")
	snapshot := worlddCharacterSnapshot(900); snapshot.ClassID = classid.Oathguard
	intent, err := outbox.EnqueueWithCompletion(identity, snapshot); if err != nil { t.Fatal(err) }

	processed, err := persistCharacterStateOutboxBatch(outbox, journal, checkpointStore, &checkpoint, store)
	if err != nil { t.Fatal(err) }
	if processed != 1 || outbox.Depth() != 0 || checkpoint.RecordID != 1 { t.Fatalf("processed=%d pending=%d checkpoint=%#v", processed, outbox.Depth(), checkpoint) }
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Snapshot.ClassID != "" { t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err) }
	if outbox.CompletionDepth() != 1 { t.Fatalf("completion depth=%d", outbox.CompletionDepth()) }
	completed := outbox.Completed(1)
	if len(completed) != 1 || completed[0] != intent { t.Fatalf("completed=%#v intent=%#v", completed, intent) }
	if reserved, ok := outbox.CompletionForCharacter(identity.ID); !ok || reserved != intent { t.Fatalf("reservation=%#v ok=%v", reserved, ok) }
}
