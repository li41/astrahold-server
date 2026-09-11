package characterstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/classid"
)

func TestStoreV9SnapshotIsClassless(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:classless-v9")
	snapshot := testSnapshot()
	record, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if record.SchemaVersion != ClasslessSchemaVersion || record.Snapshot != snapshot { t.Fatalf("record=%#v", record) }
	data, err := os.ReadFile(store.recordPath(identity.ID)); if err != nil { t.Fatal(err) }
	if bytes.Contains(data, []byte("class_id")) { t.Fatalf("v9 durable record still contains class_id: %s", data) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot != snapshot { t.Fatalf("loaded snapshot=%#v want=%#v", loaded.Snapshot, snapshot) }
}

func TestStoreV8ValidatesThenDiscardsLegacyClassID(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:legacy-class-v8")
	snapshot := testSnapshot()
	writeClassIDStoreRecord(t, store, identity, LearnedSkillsSchemaVersion, string(classid.Breaker))
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.SchemaVersion != LearnedSkillsSchemaVersion || loaded.Snapshot != snapshot { t.Fatalf("legacy loaded=%#v", loaded) }
}

func TestStoreRejectsClassIDThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct{name string; schema uint16; classID string}{
		{name: "pre-class schema carries class", schema: EquipmentSchemaVersion, classID: string(classid.Oathguard)},
		{name: "legacy class schema unknown class", schema: ClassSchemaVersion, classID: "class_guard"},
		{name: "legacy class schema whitespace class", schema: LearnedSkillsSchemaVersion, classID: " class_oathguard"},
		{name: "classless schema smuggles retired class", schema: ClasslessSchemaVersion, classID: string(classid.Oathguard)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
			identity := trusted(t, "character:class-corrupt")
			writeClassIDStoreRecord(t, store, identity, tc.schema, tc.classID)
			if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) { t.Fatalf("load err=%v", err) }
		})
	}
}

func writeClassIDStoreRecord(t *testing.T, store *Store, identity characteridentity.Binding, schema uint16, classID string) {
	t.Helper()
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: schema, CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		ClassID: classID, HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory, CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout), LearnedSkills: learnedSkillsToWire(snapshot.LearnedSkills),
	}
	data, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
}
