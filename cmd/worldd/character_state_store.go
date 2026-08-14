package main

import (
	"context"
	"fmt"
	"time"

	"github.com/li41/astrahold-server/internal/characterstate"
)

const (
	characterStateDrainEvery = 100 * time.Millisecond
	characterStateDrainBatch = 64
)

// persistCharacterStateOutboxBatch performs disk I/O outside the world owner.
// Each intent first observes the current durable revision, then uses Store.Save CAS.
// A concurrent writer using the same Store instance between Load and Save is detected
// as a revision conflict instead of being silently overwritten. The state directory is
// intentionally single-writer per worldd process; cross-process locking is out of scope.
func persistCharacterStateOutboxBatch(outbox *characterstate.Outbox, store *characterstate.Store) (int, error) {
	intents := outbox.Pending(characterStateDrainBatch)
	confirmed := 0
	for _, intent := range intents {
		current, exists, err := store.Load(intent.Identity)
		if err != nil {
			return confirmed, fmt.Errorf("load character state intent_id=%d character=%s: %w", intent.IntentID, intent.Identity.ID, err)
		}
		expectedRevision := uint64(0)
		if exists {
			expectedRevision = current.Revision
		}
		if _, err := store.Save(intent.Identity, expectedRevision, intent.Snapshot); err != nil {
			return confirmed, fmt.Errorf("save character state intent_id=%d character=%s expected_revision=%d: %w", intent.IntentID, intent.Identity.ID, expectedRevision, err)
		}
		if err := outbox.Confirm(intent.IntentID); err != nil {
			return confirmed, fmt.Errorf("confirm character state intent_id=%d: %w", intent.IntentID, err)
		}
		confirmed++
	}
	return confirmed, nil
}

func runCharacterStateStore(ctx context.Context, outbox *characterstate.Outbox, store *characterstate.Store) error {
	ticker := time.NewTicker(characterStateDrainEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for outbox.Depth() > 0 {
				processed, err := persistCharacterStateOutboxBatch(outbox, store)
				if err != nil {
					return err
				}
				if processed == 0 {
					return fmt.Errorf("character state outbox made no shutdown progress: depth=%d", outbox.Depth())
				}
			}
			return nil
		case <-ticker.C:
			if _, err := persistCharacterStateOutboxBatch(outbox, store); err != nil {
				return err
			}
		}
	}
}
