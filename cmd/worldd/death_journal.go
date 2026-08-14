package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/li41/astrahold-server/internal/deathoutcome"
)

// ingestDeathOutcomeOutboxBatch establishes the durability boundary:
// append + fsync to the journal must succeed before process-local outbox truth is confirmed.
func ingestDeathOutcomeOutboxBatch(outbox *deathoutcome.Outbox, journal *deathoutcome.Journal) (int, error) {
	events := outbox.Pending(deathOutcomeDrainBatch)
	confirmed := 0
	for _, event := range events {
		if _, err := journal.Append(event); err != nil {
			return confirmed, fmt.Errorf("append death outcome event_id=%d: %w", event.EventID, err)
		}
		if err := outbox.Confirm(event.EventID); err != nil {
			return confirmed, fmt.Errorf("confirm durable death outcome event_id=%d: %w", event.EventID, err)
		}
		confirmed++
	}
	return confirmed, nil
}

// consumeDeathOutcomeJournalBatch performs at-least-once downstream delivery.
// Consumer side effects happen before the atomic checkpoint advances.
func consumeDeathOutcomeJournalBatch(journal *deathoutcome.Journal, store *deathoutcome.CheckpointStore, checkpoint *deathoutcome.Checkpoint, consume func(deathoutcome.JournalRecord) error) (int, error) {
	records, err := journal.RecordsAfter(*checkpoint, deathOutcomeDrainBatch)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, record := range records {
		if err := consume(record); err != nil {
			return processed, fmt.Errorf("consume death outcome record_id=%d: %w", record.RecordID, err)
		}
		next, err := store.Save(journal, record)
		if err != nil {
			return processed, fmt.Errorf("checkpoint death outcome record_id=%d: %w", record.RecordID, err)
		}
		*checkpoint = next
		processed++
	}
	return processed, nil
}

func recoverDeathOutcomeJournal(journal *deathoutcome.Journal, store *deathoutcome.CheckpointStore, consume func(deathoutcome.JournalRecord) error) (deathoutcome.Checkpoint, int, error) {
	checkpoint, err := store.Load(journal)
	if err != nil {
		return deathoutcome.Checkpoint{}, 0, err
	}
	total := 0
	for {
		processed, err := consumeDeathOutcomeJournalBatch(journal, store, &checkpoint, consume)
		if err != nil {
			return deathoutcome.Checkpoint{}, total, err
		}
		total += processed
		if processed == 0 {
			return checkpoint, total, nil
		}
	}
}

func runDeathOutcomeJournal(ctx context.Context, outbox *deathoutcome.Outbox, journal *deathoutcome.Journal, store *deathoutcome.CheckpointStore, checkpoint deathoutcome.Checkpoint, consume func(deathoutcome.JournalRecord) error) error {
	ticker := time.NewTicker(deathOutcomeDrainEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for outbox.Depth() > 0 {
				processed, err := ingestDeathOutcomeOutboxBatch(outbox, journal)
				if err != nil {
					return err
				}
				if processed == 0 {
					return fmt.Errorf("death outcome outbox made no shutdown progress: depth=%d", outbox.Depth())
				}
			}
			for {
				processed, err := consumeDeathOutcomeJournalBatch(journal, store, &checkpoint, consume)
				if err != nil {
					return err
				}
				if processed == 0 {
					return nil
				}
			}
		case <-ticker.C:
			if _, err := ingestDeathOutcomeOutboxBatch(outbox, journal); err != nil {
				return err
			}
			if _, err := consumeDeathOutcomeJournalBatch(journal, store, &checkpoint, consume); err != nil {
				return err
			}
		}
	}
}

func logDeathOutcomeJournalRecord(record deathoutcome.JournalRecord) error {
	event := record.Event
	log.Printf("death outcome journal: record_id=%d event_id=%d entity=%d defeat_revision=%d context=%s defeated_tick=%d respawn_scheduled=%t spawn_point=%s spawn_class=%s due_tick=%d respawn_policy_revision=%s penalty_policy_revision=%s penalty_applied=%t checkpoint_forfeited=%t", record.RecordID, event.EventID, event.EntityID, event.DefeatRevision, event.Context, event.DefeatedTick, event.Respawn.Scheduled, event.Respawn.SpawnPointID, event.Respawn.SpawnClass, event.Respawn.DueTick, event.RespawnPolicyRevision, event.DeathPenaltyPolicyRevision, event.PenaltyTransactionApplied, event.CheckpointForfeited)
	return nil
}
