package main

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestReviveProtectionTicks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seconds  float64
		tickRate int
		want     uint64
	}{
		{name: "disabled", seconds: 0, tickRate: 20, want: 0},
		{name: "three seconds", seconds: 3, tickRate: 20, want: 60},
		{name: "ceil fractional tick", seconds: 0.051, tickRate: 20, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reviveProtectionTicks(tc.seconds, tc.tickRate)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ticks=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestReviveProtectionTicksRejectsInvalidSeconds(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := reviveProtectionTicks(value, 20); err == nil {
			t.Fatalf("value=%v should fail", value)
		}
	}
}

func TestIngestDeathOutcomeOutboxBatchPersistsBeforeConfirm(t *testing.T) {
	dir := t.TempDir()
	outbox, err := deathoutcome.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := outbox.Enqueue(worlddDeathOutcomeEvent(1, 1)); err != nil || !created {
		t.Fatalf("enqueue created=%v err=%v", created, err)
	}
	journalPath := filepath.Join(dir, "death.journal")
	journal, err := deathoutcome.OpenJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := ingestDeathOutcomeOutboxBatch(outbox, journal); err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if outbox.Depth() != 0 {
		t.Fatalf("outbox depth=%d", outbox.Depth())
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := deathoutcome.OpenJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err := reopened.RecordsAfter(reopened.InitialCheckpoint(), 64)
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	if records[0].RecordID != 1 || records[0].Event.EventID == 0 || records[0].Event.EntityID != 1 || records[0].Event.CharacterID == "" {
		t.Fatalf("record=%#v", records[0])
	}
}

func TestIngestJournalFailureLeavesOutboxPending(t *testing.T) {
	outbox, err := deathoutcome.NewOutbox(2)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := outbox.Enqueue(worlddDeathOutcomeEvent(2, 1)); err != nil || !created {
		t.Fatalf("enqueue created=%v err=%v", created, err)
	}
	journal, err := deathoutcome.OpenJournal(filepath.Join(t.TempDir(), "death.journal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if processed, err := ingestDeathOutcomeOutboxBatch(outbox, journal); processed != 0 || !errors.Is(err, deathoutcome.ErrJournalClosed) {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if outbox.Depth() != 1 {
		t.Fatalf("outbox depth=%d", outbox.Depth())
	}
}

func TestRecoverDeathOutcomeJournalReplaysOnlyAfterCheckpoint(t *testing.T) {
	dir := t.TempDir()
	journal, err := deathoutcome.OpenJournal(filepath.Join(dir, "death.journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	for i := uint64(1); i <= 2; i++ {
		event := worlddDeathOutcomeEvent(world.EntityID(i), 1)
		event.EventID = i
		if _, err := journal.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	store, err := deathoutcome.NewCheckpointStore(filepath.Join(dir, "death.checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seen []uint64
	consume := func(record deathoutcome.JournalRecord) error {
		seen = append(seen, record.RecordID)
		return nil
	}
	checkpoint, recovered, err := recoverDeathOutcomeJournal(journal, store, consume)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 2 || checkpoint.RecordID != 2 || len(seen) != 2 || seen[0] != 1 || seen[1] != 2 {
		t.Fatalf("recovered=%d checkpoint=%#v seen=%v", recovered, checkpoint, seen)
	}

	seen = nil
	checkpoint2, recovered2, err := recoverDeathOutcomeJournal(journal, store, consume)
	if err != nil {
		t.Fatal(err)
	}
	if recovered2 != 0 || checkpoint2 != checkpoint || len(seen) != 0 {
		t.Fatalf("second recovered=%d checkpoint=%#v seen=%v", recovered2, checkpoint2, seen)
	}

	third := worlddDeathOutcomeEvent(3, 1)
	third.EventID = 3
	if _, err := journal.Append(third); err != nil {
		t.Fatal(err)
	}
	seen = nil
	checkpoint3, recovered3, err := recoverDeathOutcomeJournal(journal, store, consume)
	if err != nil {
		t.Fatal(err)
	}
	if recovered3 != 1 || checkpoint3.RecordID != 3 || len(seen) != 1 || seen[0] != 3 {
		t.Fatalf("third recovered=%d checkpoint=%#v seen=%v", recovered3, checkpoint3, seen)
	}
}

func TestRunDeathOutcomeJournalShutdownDrainsOutboxAndCheckpoint(t *testing.T) {
	dir := t.TempDir()
	outbox, err := deathoutcome.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	for entity := world.EntityID(20); entity <= 21; entity++ {
		if _, created, err := outbox.Enqueue(worlddDeathOutcomeEvent(entity, 1)); err != nil || !created {
			t.Fatalf("enqueue entity=%d created=%v err=%v", entity, created, err)
		}
	}
	journal, err := deathoutcome.OpenJournal(filepath.Join(dir, "death.journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	store, err := deathoutcome.NewCheckpointStore(filepath.Join(dir, "death.checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := store.Load(journal)
	if err != nil {
		t.Fatal(err)
	}
	consumed := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runDeathOutcomeJournal(ctx, outbox, journal, store, checkpoint, func(record deathoutcome.JournalRecord) error {
		consumed++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if outbox.Depth() != 0 || consumed != 2 || journal.LastRecordID() != 2 {
		t.Fatalf("depth=%d consumed=%d last=%d", outbox.Depth(), consumed, journal.LastRecordID())
	}
	loaded, err := store.Load(journal)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RecordID != 2 {
		t.Fatalf("checkpoint=%#v", loaded)
	}
}

func worlddDeathOutcomeEvent(entityID world.EntityID, defeatRevision uint64) deathoutcome.Event {
	defeatedTick := uint64(100 + defeatRevision)
	return deathoutcome.Event{
		EntityID:                   entityID,
		CharacterID:                characteridentity.ID("character:worldd-test"),
		CharacterIdentityAssurance: characteridentity.AssuranceTrusted,
		DefeatRevision:             defeatRevision,
		Context:                    respawnpolicy.DeathContextPvE,
		DefeatedTick:               defeatedTick,
		RespawnPolicyRevision:      "respawn-worldd-test",
		DeathPenaltyPolicyRevision: "penalty-worldd-test",
		Respawn: deathoutcome.RespawnBinding{
			Scheduled:    true,
			SpawnPointID: "checkpoint",
			SpawnClass:   respawnpolicy.SpawnClassCheckpoint,
			Position:     world.Position{X: 5, Layer: 1},
			DueTick:      defeatedTick + 20,
		},
		PenaltyTransactionApplied: true,
		CheckpointForfeited:       true,
	}
}
