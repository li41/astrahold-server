package characterstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

func TestStoreCurrentSchemaRoundTripsCombatLoadout(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:loadout-v7")
	snapshot := testSnapshot()
	snapshot.CombatLoadout = mustCombatSlots(t, skillcatalog.HeavyStrike, skillcatalog.PiercingShot, skillcatalog.FireBolt)
	snapshot.LearnedSkills = mustLearnedSet(t, skillcatalog.HeavyStrike, skillcatalog.PiercingShot, skillcatalog.FireBolt)

	record, err := store.Save(identity, 0, snapshot)
	if err != nil { t.Fatal(err) }
	if record.SchemaVersion != SchemaVersion || record.Snapshot.CombatLoadout != snapshot.CombatLoadout {
		t.Fatalf("record=%#v", record)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.SchemaVersion != SchemaVersion || loaded.Snapshot.CombatLoadout != snapshot.CombatLoadout {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestStoreV6MigratesToEmptyCombatLoadout(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:loadout-v6")
	snapshot := testSnapshot()
	wire := wireRecord{
		SchemaVersion: ClassSchemaVersion,
		CharacterID: string(identity.ID), Revision: 1,
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
		Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z,
		Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory,
	}
	data, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(store.recordPath(identity.ID), append(data, '\n'), 0o600); err != nil { t.Fatal(err) }
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok { t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err) }
	if loaded.Snapshot.CombatLoadout != (skillloadout.Slots{}) {
		t.Fatalf("legacy combat loadout=%v", loaded.Snapshot.CombatLoadout)
	}
}

func TestStoreRejectsCombatLoadoutThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema uint16
		ids    []string
	}{
		{name: "legacy schema carries loadout", schema: ClassSchemaVersion, ids: []string{string(skillcatalog.HeavyStrike)}},
		{name: "loadout schema unknown skill", schema: LoadoutSchemaVersion, ids: []string{"unknown-skill"}},
		{name: "loadout schema duplicate skill", schema: LoadoutSchemaVersion, ids: []string{string(skillcatalog.FireBolt), string(skillcatalog.FireBolt)}},
		{name: "loadout schema fixed skill", schema: LoadoutSchemaVersion, ids: []string{string(skillcatalog.Guard)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil { t.Fatal(err) }
			identity := trusted(t, "character:loadout-corrupt")
			snapshot := testSnapshot()
			wire := wireRecord{
				SchemaVersion: tc.schema,
				CharacterID: string(identity.ID), Revision: 1,
				WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
				HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP,
				Defeated: snapshot.Defeated, X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z,
				Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw, Inventory: snapshot.Inventory, CombatLoadout: tc.ids,
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

func TestStoreRejectsInvalidCombatLoadoutOnSave(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	identity := trusted(t, "character:loadout-invalid-save")
	snapshot := testSnapshot()
	snapshot.CombatLoadout = skillloadout.Slots{skillcatalog.HeavyStrike, "", skillcatalog.FireBolt}
	if _, err := store.Save(identity, 0, snapshot); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("save err=%v", err)
	}
}

func TestSaveJournalCurrentSchemaRoundTripsCombatLoadout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loadout-saves.journal")
	journal, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	snapshot := testSnapshot()
	snapshot.CombatLoadout = mustCombatSlots(t, skillcatalog.Cleave, skillcatalog.RapidShot, skillcatalog.Meteor)
	snapshot.LearnedSkills = mustLearnedSet(t, skillcatalog.Cleave, skillcatalog.RapidShot, skillcatalog.Meteor)
	intent := SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-loadout"), Snapshot: snapshot}
	record, err := journal.Append(intent, 0)
	if err != nil { t.Fatal(err) }
	if err := journal.Close(); err != nil { t.Fatal(err) }

	reopened, err := OpenSaveJournal(path)
	if err != nil { t.Fatal(err) }
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil { t.Fatal(err) }
	if len(records) != 1 || records[0].RecordID != record.RecordID || records[0].Intent.Snapshot.CombatLoadout != snapshot.CombatLoadout {
		t.Fatalf("records=%#v", records)
	}
}

func TestSaveJournalV5MigratesToEmptyCombatLoadout(t *testing.T) {
	snapshot := testSnapshot()
	wireSnapshot := snapshotToSaveJournalWire(snapshot)
	wireSnapshot.CombatLoadout = nil
	wireSnapshot.LearnedSkills = nil
	wire := saveJournalWireRecord{
		SchemaVersion: ClassSaveJournalSchemaVersion,
		RecordID: 1, ExpectedRevision: 0, IntentID: 1,
		CharacterID: string(trusted(t, "character:journal-loadout-v5").ID), Snapshot: wireSnapshot,
	}
	payload, err := json.Marshal(wire)
	if err != nil { t.Fatal(err) }
	_, _, intent, err := decodeSaveJournalRecord(payload)
	if err != nil { t.Fatal(err) }
	if intent.Snapshot.CombatLoadout != (skillloadout.Slots{}) {
		t.Fatalf("legacy combat loadout=%v", intent.Snapshot.CombatLoadout)
	}
}

func TestSaveJournalRejectsCombatLoadoutThatDoesNotMatchSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema uint16
		ids    []string
	}{
		{name: "legacy schema carries loadout", schema: ClassSaveJournalSchemaVersion, ids: []string{string(skillcatalog.HeavyStrike)}},
		{name: "loadout schema unknown skill", schema: LoadoutSaveJournalSchemaVersion, ids: []string{"unknown-skill"}},
		{name: "loadout schema duplicate skill", schema: LoadoutSaveJournalSchemaVersion, ids: []string{string(skillcatalog.FireBolt), string(skillcatalog.FireBolt)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testSnapshot()
			wireSnapshot := snapshotToSaveJournalWire(snapshot)
			wireSnapshot.LearnedSkills = nil
			wireSnapshot.CombatLoadout = tc.ids
			wire := saveJournalWireRecord{
				SchemaVersion: tc.schema,
				RecordID: 1, ExpectedRevision: 0, IntentID: 1,
				CharacterID: string(trusted(t, "character:journal-loadout-corrupt").ID), Snapshot: wireSnapshot,
			}
			payload, err := json.Marshal(wire)
			if err != nil { t.Fatal(err) }
			if _, _, _, err := decodeSaveJournalRecord(payload); !errors.Is(err, ErrCorruptSaveJournal) {
				t.Fatalf("decode err=%v", err)
			}
		})
	}
}

func TestSaveJournalRejectsInvalidCombatLoadoutBeforeAppend(t *testing.T) {
	journal, err := OpenSaveJournal(filepath.Join(t.TempDir(), "invalid-loadout.journal"))
	if err != nil { t.Fatal(err) }
	defer journal.Close()
	snapshot := testSnapshot()
	snapshot.CombatLoadout = skillloadout.Slots{skillcatalog.HeavyStrike, "", skillcatalog.FireBolt}
	_, err = journal.Append(SaveIntent{IntentID: 1, Identity: trusted(t, "character:journal-invalid-loadout"), Snapshot: snapshot}, 0)
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("append err=%v", err)
	}
}

func mustCombatSlots(t *testing.T, ids ...skillcatalog.ID) skillloadout.Slots {
	t.Helper()
	slots, err := skillloadout.NewSlots(ids)
	if err != nil { t.Fatal(err) }
	return slots
}
