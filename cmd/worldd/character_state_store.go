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

// journalCharacterStateOutboxHead establishes the durable handoff boundary for exactly one
// process-local intent. The current Store revision is captured into the durable command before
// append. The outbox is Confirmed only after the full journal frame has been fsync'ed.
func journalCharacterStateOutboxHead(outbox *characterstate.Outbox, journal *characterstate.SaveJournal, store *characterstate.Store) (characterstate.SaveJournalRecord, bool, error) {
	intents := outbox.Pending(1)
	if len(intents) == 0 {
		return characterstate.SaveJournalRecord{}, false, nil
	}
	intent := intents[0]
	current, exists, err := store.Load(intent.Identity)
	if err != nil {
		return characterstate.SaveJournalRecord{}, false, fmt.Errorf("load character state before journal intent_id=%d character=%s: %w", intent.IntentID, intent.Identity.ID, err)
	}
	expectedRevision := uint64(0)
	if exists {
		expectedRevision = current.Revision
	}
	record, err := journal.Append(intent, expectedRevision)
	if err != nil {
		return characterstate.SaveJournalRecord{}, false, fmt.Errorf("append character state intent_id=%d character=%s expected_revision=%d: %w", intent.IntentID, intent.Identity.ID, expectedRevision, err)
	}
	if err := outbox.Confirm(intent.IntentID); err != nil {
		// The record is already durable. Stop the worker instead of allowing another
		// append of the same process-local intent after an unexpected outbox mismatch.
		return record, true, fmt.Errorf("confirm durable character state intent_id=%d: %w", intent.IntentID, err)
	}
	return record, true, nil
}

// applyCharacterStateJournalRecord applies one durable command exactly once with respect to
// the Store revision. A crash after Store.Save but before checkpoint leaves current revision
// at expected+1; the same snapshot then proves this record already committed. Any other
// revision/snapshot combination fails closed as a Store/journal divergence.
func applyCharacterStateJournalRecord(store *characterstate.Store, record characterstate.SaveJournalRecord) error {
	intent := record.Intent
	current, exists, err := store.Load(intent.Identity)
	if err != nil {
		return fmt.Errorf("load character state record_id=%d character=%s: %w", record.RecordID, intent.Identity.ID, err)
	}
	currentRevision := uint64(0)
	if exists {
		currentRevision = current.Revision
	}
	if currentRevision == record.ExpectedRevision {
		if _, err := store.Save(intent.Identity, record.ExpectedRevision, intent.Snapshot); err != nil {
			return fmt.Errorf("save character state record_id=%d character=%s expected_revision=%d: %w", record.RecordID, intent.Identity.ID, record.ExpectedRevision, err)
		}
		return nil
	}
	if exists && currentRevision == record.ExpectedRevision+1 && current.Snapshot == intent.Snapshot {
		return nil
	}
	return fmt.Errorf("apply character state record_id=%d character=%s journal_expected_revision=%d current_revision=%d: %w", record.RecordID, intent.Identity.ID, record.ExpectedRevision, currentRevision, characterstate.ErrRevisionConflict)
}

// consumeCharacterStateJournalBatch performs ordered durable Store application. The Store
// side effect happens before the atomic checkpoint advances. ExpectedRevision makes replay
// of the one possible applied-but-uncheckpointed head record unambiguous.
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

func drainCharacterStateJournal(
	journal *characterstate.SaveJournal,
	checkpointStore *characterstate.SaveCheckpointStore,
	checkpoint *characterstate.SaveCheckpoint,
	store *characterstate.Store,
) (int, error) {
	total := 0
	for {
		processed, err := consumeCharacterStateJournalBatch(journal, checkpointStore, checkpoint, store)
		if err != nil {
			return total, err
		}
		total += processed
		if processed == 0 {
			return total, nil
		}
	}
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
	recovered, err := drainCharacterStateJournal(journal, checkpointStore, &checkpoint, store)
	if err != nil {
		return characterstate.SaveCheckpoint{}, recovered, err
	}
	return checkpoint, recovered, nil
}

// persistCharacterStateOutboxBatch serializes each new intent as:
// Store revision read -> journal append+fsync -> outbox Confirm -> Store CAS -> checkpoint.
// Applying/checkpointing each record before journaling the next preserves the existing
// per-character sequential revision contract even when consecutive snapshots are identical.
func persistCharacterStateOutboxBatch(
	outbox *characterstate.Outbox,
	journal *characterstate.SaveJournal,
	checkpointStore *characterstate.SaveCheckpointStore,
	checkpoint *characterstate.SaveCheckpoint,
	store *characterstate.Store,
) (int, error) {
	processed := 0
	for processed < characterStateDrainBatch {
		record, ok, err := journalCharacterStateOutboxHead(outbox, journal, store)
		if err != nil {
			return processed, err
		}
		if !ok {
			return processed, nil
		}
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

func (p *characterStatePersistence) persistBatch() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// A healthy runtime normally has no journal backlog because every new record is applied
	// and checkpointed before the next append. Drain first anyway so the invariant is explicit.
	if _, err := drainCharacterStateJournal(p.journal, p.checkpointStore, &p.checkpoint, p.store); err != nil {
		return err
	}
	_, err := persistCharacterStateOutboxBatch(p.outbox, p.journal, p.checkpointStore, &p.checkpoint, p.store)
	return err
}

func (p *characterStatePersistence) flushPendingLocked() error {
	if _, err := drainCharacterStateJournal(p.journal, p.checkpointStore, &p.checkpoint, p.store); err != nil {
		return err
	}
	for p.outbox.Depth() > 0 {
		processed, err := persistCharacterStateOutboxBatch(p.outbox, p.journal, p.checkpointStore, &p.checkpoint, p.store)
		if err != nil {
			return err
		}
		if processed == 0 {
			return fmt.Errorf("character state outbox made no durable progress: depth=%d", p.outbox.Depth())
		}
	}
	_, err := drainCharacterStateJournal(p.journal, p.checkpointStore, &p.checkpoint, p.store)
	return err
}

// LoadRestore is the trusted reconnect read boundary. It first makes any process-local
// intents durable and Store-applied, catches the checkpoint up to the journal tail, and only
// then reads the Store under the same coordinator lock.
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
