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

func TestLoadRestoreFlushesPendingLeaveBeforeRead(t *testing.T) {
	store, _ := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
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
	persistence := newCharacterStatePersistence(outbox, store, worlddCharacterWorld)
	restore, ok, err := persistence.LoadRestore(identity)
	if err != nil || !ok {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
	}
	if outbox.Depth() != 0 || restore.Revision != 2 || restore.HP != 650 || restore.Transform.Position != latest.Position {
		t.Fatalf("restore=%#v depth=%d", restore, outbox.Depth())
	}
	loaded, exists, err := store.Load(identity)
	if err != nil || !exists || loaded.Revision != 2 || loaded.Snapshot != latest {
		t.Fatalf("loaded=%#v exists=%v err=%v", loaded, exists, err)
	}
}

func TestLoadRestoreRejectsWorldMismatch(t *testing.T) {
	store, _ := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
	outbox, _ := characterstate.NewOutbox(2)
	identity := worlddTrustedIdentity(t, "character:world-mismatch")
	snapshot := worlddCharacterSnapshot(800)
	snapshot.World.Revision = "old-world"
	if _, err := store.Save(identity, 0, snapshot); err != nil {
		t.Fatal(err)
	}
	persistence := newCharacterStatePersistence(outbox, store, worlddCharacterWorld)
	if _, ok, err := persistence.LoadRestore(identity); ok || !errors.Is(err, worldruntime.ErrCharacterRestoreWorldMismatch) {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestLoadRestoreAcceptsCompleteDefeatedV2Record(t *testing.T) {
	store, _ := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
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
	persistence := newCharacterStatePersistence(outbox, store, worlddCharacterWorld)
	restore, ok, err := persistence.LoadRestore(identity)
	if err != nil || !ok {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
	}
	if !restore.Defeated || restore.SchemaVersion != characterstate.SchemaVersion || restore.Respawn != snapshot.Respawn {
		t.Fatalf("restore=%#v", restore)
	}
}

func TestLoadRestoreMissingRecordUsesFreshBootstrap(t *testing.T) {
	store, _ := characterstate.Open(filepath.Join(t.TempDir(), "characters"))
	outbox, _ := characterstate.NewOutbox(2)
	persistence := newCharacterStatePersistence(outbox, store, worlddCharacterWorld)
	identity := worlddTrustedIdentity(t, "character:new")
	if restore, ok, err := persistence.LoadRestore(identity); err != nil || ok || restore != (worldruntime.CharacterRestore{}) {
		t.Fatalf("restore=%#v ok=%v err=%v", restore, ok, err)
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
	persistence := newCharacterStatePersistence(outbox, store, worlddCharacterWorld)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runCharacterStateStore(ctx, persistence); err != nil {
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
			WorldID: worlddCharacterWorld.WorldID, Revision: worlddCharacterWorld.Revision, GameplaySHA256: worlddCharacterWorld.GameplaySHA256,
		},
		HP: hp, MaxHP: 1000,
		Position: world.Position{X: 4, Y: 2, Z: -7, Layer: 1},
		Yaw:      0.75,
	}
}
