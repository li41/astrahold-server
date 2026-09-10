package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
)

func TestStoreV8RoundTripsLearnedSkills(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:learned-v8")
	snapshot := testSnapshot()
	snapshot.LearnedSkills = mustLearnedSet(t,
		skillcatalog.Haste,
		skillcatalog.FireBolt,
		skillcatalog.RandomTeleport,
		skillcatalog.HeavyStrike,
		skillcatalog.StrongPhysique,
	)

	record, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if record.SchemaVersion != LearnedSkillsSchemaVersion || record.Snapshot.LearnedSkills != snapshot.LearnedSkills {
		t.Fatalf("record=%#v", record)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.SchemaVersion != LearnedSkillsSchemaVersion || loaded.Snapshot.LearnedSkills != snapshot.LearnedSkills {
		t.Fatalf("loaded=%#v", loaded)
	}

	data, err := os.ReadFile(store.recordPath(identity.ID))
	if err != nil { t.Fatal(err) }
	var wire wireRecord
	if err := json.Unmarshal(data, &wire); err != nil { t.Fatal(err) }
	wantWire := []string{
		string(skillcatalog.RandomTeleport),
		string(skillcatalog.HeavyStrike),
		string(skillcatalog.FireBolt),
		string(skillcatalog.StrongPhysique),
		string(skillcatalog.Haste),
	}
	if !reflect.DeepEqual(wire.LearnedSkills, wantWire) {
		t.Fatalf("wire learned skills=%v want=%v", wire.LearnedSkills, wantWire)
	}
}

func TestStoreV7MigratesToEmptyLearnedSkills(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:learned-v7")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: LoadoutSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z,
		Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory,
		CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout),
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.LearnedSkills != (learnedskills.Set{}) {
		t.Fatalf("legacy learned skills=%v", loaded.Snapshot.LearnedSkills.IDs())
	}
}

func TestStoreRejectsLearnedSkillsThatDoNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema uint16
		ids    []string
	}{
		{name: "legacy schema carries learned skills", schema: LoadoutSchemaVersion, ids: []string{string(skillcatalog.HeavyStrike)}},
		{name: "current schema unknown skill", schema: LearnedSkillsSchemaVersion, ids: []string{"unknown-skill"}},
		{name: "current schema duplicate skill", schema: LearnedSkillsSchemaVersion, ids: []string{string(skillcatalog.FireBolt), string(skillcatalog.FireBolt)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil { t.Fatal(err) }
			identity := trusted(t, "character:learned-corrupt")
			snapshot := testSnapshot()
			wire := wireRecord{
				SchemaVersion: tc.schema,
				CharacterID: string(identity.ID), Revision: 1,
				WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
				HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
				Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z,
				Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory,
				CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout), LearnedSkills: tc.ids,
			}
			data, err := json.Marshal(wire)
			if err != nil { t.Fatal(err) }
			if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
			if _, _, err := store.Load(identity); !errors.Is(err, ErrCorruptRecord) {
				t.Fatalf("load err=%v", err)
			}
		})
	}
}

func TestSaveJournalV7RoundTripsLearnedSkills(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned-saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.LearnedSkills = mustLearnedSet(t, skillcatalog.Cleave, skillcatalog.Haste, skillcatalog.Heal, skillcatalog.Meteor)
	intent := SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-learned"), Snapshot: snapshot}
	record, err := journal.Append(intent, 0)
	if err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].RecordID != record.RecordID || records[0].Intent.Snapshot.LearnedSkills != snapshot.LearnedSkills {
		t.Fatalf("records=%#v", records)
	}
}

func TestSaveJournalV6MigratesToEmptyLearnedSkills(t *testing.T) {
	snapshot := testSnapshot()
	wireSnapshot := snapshotToSaveJournalWire(snapshot)
	wireSnapshot.LearnedSkills = nil
	wire := saveJournalWireRecord{
		SchemaVersion: LoadoutSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 1,
		CharacterID: string(trusted(t, "character:journal-learned-v6").ID), Snapshot: wireSnapshot,
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	_, _, intent, err := decodeSaveJournalRecord(payload)
	if err != nil { t.Fatal(err) }
	if intent.Snapshot.LearnedSkills != (learnedskills.Set{}) {
		t.Fatalf("legacy learned skills=%v", intent.Snapshot.LearnedSkills.IDs())
	}
}

func TestSaveJournalRejectsLearnedSkillsThatDoNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema uint16
		ids    []string
	}{
		{name: "legacy schema carries learned skills", schema: LoadoutSaveJournalSchemaVersion, ids: []string{string(skillcatalog.HeavyStrike)}},
		{name: "current schema unknown skill", schema: LearnedSkillsSaveJournalSchemaVersion, ids: []string{"unknown-skill"}},
		{name: "current schema duplicate skill", schema: LearnedSkillsSaveJournalSchemaVersion, ids: []string{string(skillcatalog.FireBolt), string(skillcatalog.FireBolt)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testSnapshot()
			wireSnapshot := snapshotToSaveJournalWire(snapshot)
			wireSnapshot.LearnedSkills = tc.ids
			wire := saveJournalWireRecord{
				SchemaVersion: tc.schema,
				RecordID: 1, ExpectedRevision: 0, IntentID: 1,
				CharacterID: string(trusted(t, "character:journal-learned-corrupt").ID), Snapshot: wireSnapshot,
			}
			payload, err := json.Marshal(wire)
			if err != nil { t.Fatal(err) }
			if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
				t.Fatalf("decode err=%v", err)
			}
		})
	}
}

func mustLearnedSet(t *testing.T, ids ...skillcatalog.ID) learnedskills.Set {
	t.Helper()
	set, err := learnedskills.NewSet(ids)
	if err != nil { t.Fatal(err) }
	return set
}
