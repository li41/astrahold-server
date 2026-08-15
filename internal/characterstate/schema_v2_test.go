package characterstate

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestStoreReadsLegacyV1AliveRecord(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:legacy-alive")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: LegacySchemaVersion,
		CharacterID: string(identity.ID), Revision: 9,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, Defeated: false,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	data, _ := json.Marshal(wire)
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	if loaded.SchemaVersion != LegacySchemaVersion || loaded.Revision != 9 || loaded.Snapshot != snapshot {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestStoreReadsLegacyV1DefeatedWithoutInventingRespawnTruth(t *testing.T) {
	store, _ := Open(t.TempDir())
	identity := trusted(t, "character:legacy-defeated")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: LegacySchemaVersion,
		CharacterID: string(identity.ID), Revision: 4,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: 0, MaxHP: snapshot.MaxHP, Defeated: true,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	data, _ := json.Marshal(wire)
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	if loaded.SchemaVersion != LegacySchemaVersion || !loaded.Snapshot.Defeated || loaded.Snapshot.Respawn != (DefeatedRespawn{}) {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestStoreV2DefeatedRoundTripAllContexts(t *testing.T) {
	for _, context := range []respawnpolicy.DeathContext{respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege} {
		t.Run(string(context), func(t *testing.T) {
			root := t.TempDir()
			store, _ := Open(root)
			identity := trusted(t, "character:v2-"+string(context))
			snapshot := testSnapshot()
			snapshot.HP = 0
			snapshot.Defeated = true
			snapshot.Respawn = DefeatedRespawn{
				Context: context, SpawnPointID: "bound", SpawnClass: respawnpolicy.SpawnClassSafe,
				Position: world.Position{X: 8, Y: 0, Z: -2, Layer: 1}, RemainingTicks: 17, CheckpointID: "checkpoint",
			}
			saved, err := store.Save(identity, 0, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if saved.SchemaVersion != SchemaVersion {
				t.Fatalf("schema=%d", saved.SchemaVersion)
			}
			reopened, _ := Open(root)
			loaded, ok, err := reopened.Load(identity)
			if err != nil || !ok || loaded != saved {
				t.Fatalf("loaded=%#v saved=%#v ok=%v err=%v", loaded, saved, ok, err)
			}
		})
	}
}

func TestStoreV2DefeatedRequiresCompleteRespawnMetadata(t *testing.T) {
	store, _ := Open(t.TempDir())
	identity := trusted(t, "character:partial")
	snapshot := testSnapshot()
	snapshot.HP = 0
	snapshot.Defeated = true
	if _, err := store.Save(identity, 0, snapshot); err == nil {
		t.Fatal("incomplete defeated snapshot unexpectedly saved")
	}
}
