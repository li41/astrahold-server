package characterstate

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/world"
)

const testGameplaySHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestStoreCreateUpdateReopenAndCAS(t *testing.T) {
	root := filepath.Join(t.TempDir(), "characters")
	store, err := Open(root)
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:alpha")
	if _, ok, err := store.Load(identity); err != nil || ok { t.Fatalf("missing load ok=%v err=%v", ok, err) }
	firstSnapshot := testSnapshot()
	first, err := store.Save(identity, 0, firstSnapshot)
	if err != nil { t.Fatal(err) }
	if first.Revision != 1 || first.CharacterID != identity.ID || first.Snapshot != firstSnapshot { t.Fatalf("first=%#v", first) }
	updatedSnapshot := firstSnapshot
	updatedSnapshot.HP = 700
	updatedSnapshot.MP = 40
	updatedSnapshot.Position = world.Position{X: 12, Y: 3, Z: -4, Layer: 2}
	updatedSnapshot.Yaw = 1.25
	second, err := store.Save(identity, first.Revision, updatedSnapshot)
	if err != nil { t.Fatal(err) }
	if second.Revision != 2 || second.Snapshot != updatedSnapshot { t.Fatalf("second=%#v", second) }
	if _, err := store.Save(identity, 1, firstSnapshot); !errors.Is(err, ErrRevisionConflict) { t.Fatalf("stale save err=%v", err) }
	current, ok, err := store.Load(identity)
	if err != nil || !ok || current != second { t.Fatalf("current=%#v ok=%v err=%v", current, ok, err) }
	reopened, err := Open(root)
	if err != nil { t.Fatal(err) }
	loaded, ok, err := reopened.Load(identity)
	if err != nil || !ok || loaded != second { t.Fatalf("reopened=%#v ok=%v err=%v", loaded, ok, err) }
}

func TestStoreMigratesV2RecordToFullMP(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:v2-mp-migration")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: RespawnSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, Defeated: false,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.MP != LegacyDefaultMaxMP || loaded.Snapshot.MaxMP != LegacyDefaultMaxMP {
		t.Fatalf("migrated mp=%d/%d", loaded.Snapshot.MP, loaded.Snapshot.MaxMP)
	}
}

func TestStoreCreateOnlyConflictDoesNotOverwrite(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:create-only")
	first, err := store.Save(identity, 0, testSnapshot()); if err != nil { t.Fatal(err) }
	changed := testSnapshot(); changed.HP = 500
	if _, err := store.Save(identity, 0, changed); !errors.Is(err, ErrRevisionConflict) { t.Fatalf("create-only err=%v", err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded != first { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
}

func TestStoreRejectsEphemeralIdentity(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	ephemeral, err := characteridentity.NewEphemeral(); if err != nil { t.Fatal(err) }
	if _, err := store.Save(ephemeral, 0, testSnapshot()); !errors.Is(err, ErrIdentityNotDurable) { t.Fatalf("save err=%v", err) }
	if _, _, err := store.Load(ephemeral); !errors.Is(err, ErrIdentityNotDurable) { t.Fatalf("load err=%v", err) }
}

func TestStoreRejectsInvalidSnapshots(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:invalid")
	tests := []Snapshot{
		{},
		func() Snapshot { s := testSnapshot(); s.World.GameplaySHA256 = "BAD"; return s }(),
		func() Snapshot { s := testSnapshot(); s.MaxHP = 0; return s }(),
		func() Snapshot { s := testSnapshot(); s.HP = s.MaxHP + 1; return s }(),
		func() Snapshot { s := testSnapshot(); s.MaxMP = 0; return s }(),
		func() Snapshot { s := testSnapshot(); s.MP = s.MaxMP + 1; return s }(),
		func() Snapshot { s := testSnapshot(); s.Defeated = true; s.HP = 1; return s }(),
		func() Snapshot { s := testSnapshot(); s.HP = 0; s.Defeated = false; return s }(),
		func() Snapshot { s := testSnapshot(); s.Position.X = float32(math.NaN()); return s }(),
		func() Snapshot { s := testSnapshot(); s.Yaw = float32(math.Inf(1)); return s }(),
	}
	for i, snapshot := range tests {
		if _, err := store.Save(identity, 0, snapshot); !errors.Is(err, ErrInvalidSnapshot) { t.Fatalf("case=%d err=%v", i, err) }
	}
}

func TestStoreFailsClosedOnUnknownTrailingOrMismatchedRecord(t *testing.T) {
	for _, tc := range []struct { name string; mutate func(map[string]any) []byte }{
		{name:"unknown field", mutate:func(wire map[string]any) []byte { wire["unexpected"]=true; data,_:=json.Marshal(wire); return append(data,'\n') }},
		{name:"trailing json", mutate:func(wire map[string]any) []byte { data,_:=json.Marshal(wire); return append(append(data,'\n'), []byte("{}\n")...) }},
		{name:"mismatched id", mutate:func(wire map[string]any) []byte { wire["character_id"]="character:other"; data,_:=json.Marshal(wire); return append(data,'\n') }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
			identity := trusted(t, "character:corrupt")
			if _, err := store.Save(identity, 0, testSnapshot()); err != nil { t.Fatal(err) }
			path := store.recordPath(identity.ID)
			data, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
			var wire map[string]any; if err := json.Unmarshal(data,&wire); err != nil { t.Fatal(err) }
			if err := os.WriteFile(path, tc.mutate(wire), 0o600); err != nil { t.Fatal(err) }
			if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) { t.Fatalf("load err=%v", err) }
		})
	}
}

func TestStoreRevisionOverflowDoesNotAdvanceTruth(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:max-revision")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: SchemaVersion, CharacterID: string(identity.ID), Revision: ^uint64(0),
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP, Defeated: snapshot.Defeated,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	data, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data,'\n'), 0o600); err != nil { t.Fatal(err) }
	if _, err := store.Save(identity, ^uint64(0), snapshot); !errors.Is(err, ErrRevisionOverflow) { t.Fatalf("overflow err=%v", err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != ^uint64(0) { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
}

func TestStoreUsesHashedFilenamesAndSeparatesCharacters(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	firstIdentity := trusted(t, "character:alpha")
	secondIdentity := trusted(t, "character:beta")
	if _, err := store.Save(firstIdentity, 0, testSnapshot()); err != nil { t.Fatal(err) }
	secondSnapshot := testSnapshot(); secondSnapshot.HP = 600
	if _, err := store.Save(secondIdentity, 0, secondSnapshot); err != nil { t.Fatal(err) }
	if filepath.Base(store.recordPath(firstIdentity.ID)) == filepath.Base(store.recordPath(secondIdentity.ID)) { t.Fatal("distinct character ids mapped to same record path") }
	entries, err := os.ReadDir(store.Path()); if err != nil { t.Fatal(err) }
	if len(entries) != 2 { t.Fatalf("entries=%d", len(entries)) }
	for _, entry := range entries { if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" { t.Fatalf("unexpected entry=%s", entry.Name()) } }
}

func trusted(t *testing.T, id string) characteridentity.Binding {
	t.Helper()
	binding, err := characteridentity.NewTrusted(id); if err != nil { t.Fatal(err) }
	return binding
}

func testSnapshot() Snapshot {
	return Snapshot{
		World: WorldRef{WorldID:"castle-sandbox",Revision:"s3d-001",GameplaySHA256:testGameplaySHA},
		HP:900, MaxHP:1000, MP:100, MaxMP:100, Defeated:false,
		Position:world.Position{X:4,Y:2,Z:-7,Layer:1}, Yaw:0.75,
	}
}
