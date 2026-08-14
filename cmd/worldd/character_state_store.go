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

// characterStatePersistence serializes durable save batches and reconnect restore reads.
// A restore first flushes all save intents that are already pending, then loads the durable
// record under the same coordinator lock. This prevents a fast reconnect from observing a
// stale Store record merely because the preceding leave intent had not reached disk yet.
// World-owner enqueue remains non-blocking and performs no file I/O.
type characterStatePersistence struct {
	mu      sync.Mutex
	outbox  *characterstate.Outbox
	store   *characterstate.Store
	world   protocol.WorldIdentity
}

func newCharacterStatePersistence(outbox *characterstate.Outbox, store *characterstate.Store, world protocol.WorldIdentity) *characterStatePersistence {
	return &characterStatePersistence{outbox: outbox, store: store, world: world}
}

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

func (p *characterStatePersistence) persistBatch() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return persistCharacterStateOutboxBatch(p.outbox, p.store)
}

func (p *characterStatePersistence) flushPendingLocked() error {
	for p.outbox.Depth() > 0 {
		processed, err := persistCharacterStateOutboxBatch(p.outbox, p.store)
		if err != nil {
			return err
		}
		if processed == 0 {
			return fmt.Errorf("character state outbox made no progress: depth=%d", p.outbox.Depth())
		}
	}
	return nil
}

// LoadRestore is the trusted reconnect read boundary. It flushes save intents that are
// already pending before reading the Store, then applies the same exact-world/identity/
// defeated validation that the transport and Runtime will enforce again.
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
			if _, err := persistence.persistBatch(); err != nil {
				return err
			}
		}
	}
}
