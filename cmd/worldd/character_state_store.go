package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	characterStateDrainEvery = 100 * time.Millisecond
	characterStateDrainBatch = 64
)

// characterStatePersistence serializes save-journal ingest, durable Store application,
// checkpoints, and reconnect restore reads. World-owner enqueue remains non-blocking and
// performs no file I/O.
type characterStatePersistence struct {
	mu              sync.Mutex
	outbox          *characterstate.Outbox
	journal         *characterstate.SaveJournal
	checkpointStore *characterstate.SaveCheckpointStore
	checkpoint      characterstate.SaveCheckpoint
	store           *characterstate.Store
	world           protocol.WorldIdentity
}

func newCharacterStatePersistence(
	outbox *characterstate.Outbox,
	journal *characterstate.SaveJournal,
	checkpointStore *characterstate.SaveCheckpointStore,
	checkpoint characterstate.SaveCheckpoint,
	store *characterstate.Store,
	world protocol.WorldIdentity,
) *characterStatePersistence {
	return &characterStatePersistence{
		outbox: outbox, journal: journal, checkpointStore: checkpointStore,
		checkpoint: checkpoint, store: store, world: world,
	}
}

// ingestCharacterStateOutboxBatch establishes the durable handoff boundary. A process-local
// intent is confirmed only after its full journal frame has been appended and fsync'ed.
func ingestCharacterStateOutboxBatch(outbox *characterstate.Outbox, journal *characterstate.SaveJournal) (int, error) {
	intents := outbox.Pending(characterStateDrainBatch)
	confirmed := 0
	for _, intent := range intents {
		if _, err := journal.Append(intent); err != nil {
			return confirmed, fmt.Errorf("append character state intent_id=%d character=%s: %w", intent.IntentID, intent.Identity.ID, err)
		}
		if err := outbox.Confirm(intent.IntentID); err != nil {
			// The journal record is already durable. Stop rather than appending the same
			// process-local intent repeatedly after an unexpected outbox state mismatch.
			return confirmed, fmt.Errorf("confirm durable character state intent_id=%d: %w", intent.IntentID, err)
		}
		confirmed++
	}
	return confirmed, nil
}

// applyCharacterStateJournalRecord applies one durable save intent to the optimistic Store.
// If the exact snapshot is already current, the side effect is treated as idempotently
// complete; this covers a crash after Store.Save but before checkpoint advancement.
func applyCharacterStateJournalRecord(store *characterstate.Store, record characterstate.SaveJournalRecord) error {
	intent := record.Intent
	current, exists, err := store.Load(intent.Identity)
	if err != nil {
		return fmt.Errorf("load character state record_id=%d character=%s: %w", record.RecordID, intent.Identity.ID, err)
	}
	if exists && current.Snapshot == intent.Snapshot {
		return nil
	}
	expectedRevision := uint64(0)
	if exists {
		expectedRevision = current.Revision
	}
	if _, err := store.Save(intent.Identity, expectedRevision, intent.Snapshot); err != nil {
		return fmt.Errorf("save character state record_id=%d character=%s expected_revision=%d: %w", record.RecordID, intent.Identity.ID, expectedRevision, err)
	}
	return nil
}

// consumeCharacterStateJournalBatch performs at-least-once Store application. The Store
// side effect happens before the atomic checkpoint advances.
func consumeCharacterStateJournalBatch(
	journal *characterstate.SaveJournal,
	checkpointStore *characterstate.SaveCheckpointStore,
	checkpoint *characterstate.SaveCheckpoint,
	store *characterstate.Store,
) (int, error) {
	records, err := journal.RecordsAfter(*checkpoint, characterStateDrainBatch)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, record := range records {
		if err := applyCharacterStateJournalRecord(store, record); err != nil {
			return processed, err
		}
		next, err := checkpointStore.Save(journal, record)
		if err != nil {
			return processed, fmt.Errorf("checkpoint character state record_id=%d: %w", record.RecordID, err)
		}
		*checkpoint = next
		processed++
	}
	return processed, nil
}

func recoverCharacterStateSaveJournal(
	journal *characterstate.SaveJournal,
	checkpointStore *characterstate.SaveCheckpointStore,
	store *characterstate.Store,
) (characterstate.SaveCheckpoint, int, error) {
	checkpoint, err := checkpointStore.Load(journal)
	if err != nil {
		return characterstate.SaveCheckpoint{}, 0, err
	}
	total := 0
	for {
		processed, err := consumeCharacterStateJournalBatch(journal, checkpointStore, &checkpoint, store)
		if err != nil {
			return characterstate.SaveCheckpoint{}, total, err
		}
		total += processed
		if processed == 0 {
			return checkpoint, total, nil
		}
	}
}

func (p *characterStatePersistence) persistBatch() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := ingestCharacterStateOutboxBatch(p.outbox, p.journal); err != nil {
		return err
	}
	_, err := consumeCharacterStateJournalBatch(p.journal, p.checkpointStore, &p.checkpoint, p.store)
	return err
}

func (p *characterStatePersistence) flushPendingLocked() error {
	for p.outbox.Depth() > 0 {
		processed, err := ingestCharacterStateOutboxBatch(p.outbox, p.journal)
		if err != nil {
			return err
		}
		if processed == 0 {
			return fmt.Errorf("character state outbox made no journal progress: depth=%d", p.outbox.Depth())
		}
	}
	for {
		processed, err := consumeCharacterStateJournalBatch(p.journal, p.checkpointStore, &p.checkpoint, p.store)
		if err != nil {
			return err
		}
		if processed == 0 {
			return nil
		}
	}
}

// LoadRestore is the trusted reconnect read boundary. It first makes any process-local
// intents durable in the journal, then catches the Store checkpoint up to the journal tail,
// and only then reads the Store under the same coordinator lock.
func (p *characterStatePersistence) LoadRestore(identity characteridentity.Binding) (worldruntime.CharacterRestore, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if identity.Assurance != characteridentity.AssuranceTrusted || !identity.Valid() {
		return worldruntime.CharacterRestore{}, false, worldruntime.ErrCharacterRestoreRequiresTrustedIdentity
	}
	if err := p.flushPendingLocked(); err != nil {
		return worldruntime.CharacterRestore{}, false, fmt.Errorf("flush character state before restore character=%s: %w", identity.ID, err)
	}
	record, exists, err := p.store.Load(identity)
	if err != nil {
		return worldruntime.CharacterRestore{}, false, fmt.Errorf("load character restore character=%s: %w", identity.ID, err)
	}
	if !exists {
		return worldruntime.CharacterRestore{}, false, nil
	}
	restore := worldruntime.CharacterRestoreFromRecord(record)
	if err := worldruntime.ValidateCharacterRestore(identity, restore, p.world); err != nil {
		return worldruntime.CharacterRestore{}, false, err
	}
	return restore, true, nil
}

func runCharacterStateStore(ctx context.Context, persistence *characterStatePersistence) error {
	ticker := time.NewTicker(characterStateDrainEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			persistence.mu.Lock()
			err := persistence.flushPendingLocked()
			persistence.mu.Unlock()
			return err
		case <-ticker.C:
			if err := persistence.persistBatch(); err != nil {
				return err
			}
		}
	}
}
