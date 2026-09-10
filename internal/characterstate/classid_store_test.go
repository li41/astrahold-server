package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/classid"
)

func TestStoreV6RoundTripsCanonicalClassID(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:class-v6")
	snapshot := testSnapshot()
	snapshot.ClassID = classid.Oathguard
	record, err := store.Save(identity, 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != SchemaVersion || record.Snapshot.ClassID != classid.Oathguard {
		t.Fatalf("record=%#v", record)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	if loaded.SchemaVersion != SchemaVersion || loaded.Snapshot.ClassID != classid.Oathguard {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestStoreV5MigratesToUnassignedClassID(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:class-v5")
	writeClassIDStoreRecord(t, store, identity, EquipmentSchemaVersion, "")
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	if loaded.SchemaVersion != EquipmentSchemaVersion || loaded.Snapshot.ClassID != "" {
		t.Fatalf("legacy loaded=%#v", loaded)
	}
}

func TestStoreRejectsClassIDThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  uint16
		classID string
	}{
		{name: "legacy schema carries class", schema: EquipmentSchemaVersion, classID: string(classid.Oathguard)},
		{name: "current schema unknown class", schema: ClassSchemaVersion, classID: "class_guard"},
		{name: "current schema whitespace class", schema: ClassSchemaVersion, classID: " class_oathguard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			identity := trusted(t, "character:class-corrupt")
			writeClassIDStoreRecord(t, store, identity, tc.schema, tc.classID)
			if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) {
				t.Fatalf("load err=%v", err)
			}
		})
	}
}

func TestStoreRejectsInvalidClassIDOnSave(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:class-invalid-save")
	snapshot := testSnapshot()
	snapshot.ClassID = classid.ID("class_oathguard ")
	if _, err := store.Save(identity, 0, snapshot); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("save err=%v", err)
	}
}

func writeClassIDStoreRecord(t *testing.T, store *Store, identity characteridentity.Binding, schema uint16, classID string) {
	t.Helper()
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: schema,
		CharacterID: string(identity.ID),
		Revision: 1,
		WorldID: snapshot.World.WorldID,
		WorldRevision: snapshot.World.Revision,
		GameplaySHA256: snapshot.World.GameplaySHA256,
		ClassID: classID,
		HP: snapshot.HP,
		MaxHP: snapshot.MaxHP,
		MP: snapshot.MP,
		MaxMP: snapshot.MaxMP,
		Defeated: snapshot.Defeated,
		X: snapshot.Position.X,
		Y: snapshot.Position.Y,
		Z: snapshot.Position.Z,
		Layer: snapshot.Position.Layer,
		Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
