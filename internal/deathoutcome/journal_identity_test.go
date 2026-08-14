package deathoutcome

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestJournalV2PreservesCharacterIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "death.journal")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	event := journalTestEvent(1, 40, 1)
	event.CharacterID = characteridentity.ID("character:alpha")
	record, err := journal.Append(event)
	if err != nil {
		t.Fatal(err)
	}
	initial := journal.InitialCheckpoint()
	records, err := journal.RecordsAfter(initial, 0)
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	if records[0].RecordID != record.RecordID || records[0].Event.CharacterIdentity() != event.CharacterIdentity() {
		t.Fatalf("record=%#v event=%#v", records[0], event)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil || len(recovered) != 1 || recovered[0].Event.CharacterIdentity() != event.CharacterIdentity() {
		t.Fatalf("recovered=%#v err=%v", recovered, err)
	}
}

func TestJournalReadsLegacyV1ThenAppendsV2WithoutInventingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "death.journal")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	journalID := journal.ID()
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	legacy := Event{
		EventID:        1,
		EntityID:       51,
		DefeatRevision: 1,
		Context:        respawnpolicy.DeathContextPvP,
		DefeatedTick:   100,
	}
	payload, err := json.Marshal(journalWireRecord{
		SchemaVersion: legacyJournalSchemaVersion,
		RecordID:      1,
		Event:         eventToWire(legacy),
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(makeFrame(payload)); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID() != journalID || reopened.LastRecordID() != 1 {
		t.Fatalf("id=%s last=%d", reopened.ID(), reopened.LastRecordID())
	}
	legacyRecords, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 0)
	if err != nil || len(legacyRecords) != 1 {
		t.Fatalf("legacy=%#v err=%v", legacyRecords, err)
	}
	if legacyRecords[0].Event.CharacterID != "" || legacyRecords[0].Event.CharacterIdentityAssurance != "" {
		t.Fatalf("legacy identity must remain absent: %#v", legacyRecords[0].Event)
	}

	v2 := Event{
		EventID:                    2,
		EntityID:                   52,
		CharacterID:                characteridentity.ID("character:returning"),
		CharacterIdentityAssurance: characteridentity.AssuranceTrusted,
		DefeatRevision:             1,
		Context:                    respawnpolicy.DeathContextPvE,
		DefeatedTick:               200,
		RespawnPolicyRevision:      "respawn-v2",
		DeathPenaltyPolicyRevision: "penalty-v2",
		Respawn: RespawnBinding{
			Scheduled:    true,
			SpawnPointID: "safe",
			SpawnClass:   respawnpolicy.SpawnClassSafe,
			Position:     world.Position{X: 1, Layer: 0},
			DueTick:      220,
		},
	}
	second, err := reopened.Append(v2)
	if err != nil {
		t.Fatal(err)
	}
	if second.RecordID != 2 {
		t.Fatalf("record id=%d", second.RecordID)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	all, err := again.RecordsAfter(again.InitialCheckpoint(), 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("records=%#v err=%v", all, err)
	}
	if all[0].Event.CharacterID != "" || all[1].Event.CharacterIdentity() != v2.CharacterIdentity() {
		t.Fatalf("mixed records=%#v", all)
	}
}
