package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreCurrentRoundTripsMapID(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:map-current"); snapshot := testSnapshot(); snapshot.World.MapID = "map0"
	saved, err := store.Save(identity, 0, snapshot); if err != nil { t.Fatal(err) }
	if saved.SchemaVersion != MapSchemaVersion { t.Fatalf("schema=%d want=%d", saved.SchemaVersion, MapSchemaVersion) }
	loaded, ok, err := store.Load(identity); if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.World.MapID != "map0" { t.Fatalf("map=%q want=map0", loaded.Snapshot.World.MapID) }
}

func TestStoreV13MissingMapMigratesToMap1(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:map-v13-migrate"); snapshot := testSnapshot()
	wire := wireRecord{SchemaVersion: AppearanceSchemaVersion, CharacterID: string(identity.ID), Revision: 1, WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256, SkinID: string(snapshot.SkinID), HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP, Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory, CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout), LearnedSkills: learnedSkillsToWire(snapshot.LearnedSkills), PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats)}
	data, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity); if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.World.MapID != LegacyDefaultMapID { t.Fatalf("legacy map=%q want=%q", loaded.Snapshot.World.MapID, LegacyDefaultMapID) }
}

func TestStoreCurrentMissingMapFailsClosed(t *testing.T) {
	store, err := Open(t.TempDir()); if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:map-current-missing"); snapshot := testSnapshot()
	wire := wireRecord{SchemaVersion: MapSchemaVersion, CharacterID: string(identity.ID), Revision: 1, WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256, SkinID: string(snapshot.SkinID), HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP, Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory, CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout), LearnedSkills: learnedSkillsToWire(snapshot.LearnedSkills), PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats)}
	data, err := json.Marshal(wire); if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) { t.Fatalf("missing current map err=%v", err) }
}

func TestSaveJournalMapRoundTripAndLegacyFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map.journal"); journal, err := OpenSaveJournal(path); if err != nil { t.Fatal(err) }
	snapshot := testSnapshot(); snapshot.World.MapID = "map0"
	intent := SaveIntent{IntentID: 71, Identity: trusted(t, "character:map-journal"), Snapshot: snapshot}
	if _, err := journal.Append(intent, 0); err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }
	reopened, err := OpenSaveJournal(path); if err != nil { t.Fatal(err) }; defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0); if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].Intent.Snapshot.World.MapID != "map0" { t.Fatalf("records=%#v", records) }
	legacyWire := saveJournalWireRecord{SchemaVersion: AppearanceSaveJournalSchemaVersion, RecordID: 1, ExpectedRevision: 0, IntentID: 72, CharacterID: string(trusted(t, "character:map-journal-legacy").ID), Snapshot: snapshotToSaveJournalWire(testSnapshot())}
	legacyWire.Snapshot.MapID = ""
	payload, err := json.Marshal(legacyWire); if err != nil { t.Fatal(err) }
	_, _, decoded, err := decodeSaveJournalRecord(payload); if err != nil { t.Fatal(err) }
	if decoded.Snapshot.World.MapID != LegacyDefaultMapID { t.Fatalf("legacy journal map=%q want=%q", decoded.Snapshot.World.MapID, LegacyDefaultMapID) }
}
