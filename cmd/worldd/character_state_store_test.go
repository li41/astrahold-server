package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/world"
)

const worlddCharacterStateSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPersistCharacterStateOutboxBatchSavesBeforeConfirm(t *testing.T) {
	store, err := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
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
	processed, err := persistCharacterStateOutboxBatch(outbox, store)
	if err != nil || processed != 1 || outbox.Depth() != 0 {
		t.Fatalf("processed=%d depth=%d err=%v", processed, outbox.Depth(), err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v ok=%v err=%v intent=%#v", loaded, ok, err, intent)
	}
}

func TestPersistCharacterStateSequentialIntentsAdvanceRevision(t *testing.T) {
	store, err := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
	if err != nil {
		t.Fatal(err)
	}
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
	processed, err := persistCharacterStateOutboxBatch(outbox, store)
	if err != nil || processed != 2 || outbox.Depth() != 0 {
		t.Fatalf("processed=%d depth=%d err=%v", processed, outbox.Depth(), err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 2 || loaded.Snapshot != second {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
}

func TestRunCharacterStateStoreShutdownDrainsPending(t *testing.T) {
	store, err := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
	if err != nil {
		t.Fatal(err)
	}
	outbox, _ := characterstate.NewOutbox(2)
	identity := worlddTrustedIdentity(t, "character:shutdown")
	if _, err := outbox.Enqueue(identity, worlddCharacterSnapshot(800)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runCharacterStateStore(ctx, outbox, store); err != nil {
		t.Fatal(err)
	}
	if outbox.Depth() != 0 {
		t.Fatalf("depth=%d", outbox.Depth())
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
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
			WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: worlddCharacterStateSHA,
		},
		HP: hp, MaxHP: 1000,
		Position: world.Position{X: 4, Y: 2, Z: -7, Layer: 1},
		Yaw:      0.75,
	}
}
