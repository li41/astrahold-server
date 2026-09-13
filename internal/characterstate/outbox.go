package characterstate

import (
	"errors"
	"fmt"
	"sync"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

var (
	ErrInvalidOutboxCapacity = errors.New("characterstate: invalid outbox capacity")
	ErrSaveOutboxFull        = errors.New("characterstate: save outbox full")
	ErrSaveIntentOverflow    = errors.New("characterstate: save intent id overflow")
	ErrUnknownSaveIntent     = errors.New("characterstate: unknown save intent")
	ErrSaveConfirmOutOfOrder = errors.New("characterstate: save confirm out of order")
)

type SaveIntent struct {
	IntentID uint64
	Identity characteridentity.Binding
	Snapshot Snapshot
}

type Outbox struct {
	mu           sync.Mutex
	capacity     int
	nextIntentID uint64
	pending      []SaveIntent
}

func NewOutbox(capacity int) (*Outbox, error) {
	if capacity <= 0 {
		return nil, ErrInvalidOutboxCapacity
	}
	return &Outbox{capacity: capacity, pending: make([]SaveIntent, 0, capacity)}, nil
}

func (o *Outbox) Capacity() int { return o.capacity }

func (o *Outbox) Depth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pending)
}

func (o *Outbox) Enqueue(identity characteridentity.Binding, snapshot Snapshot) (SaveIntent, error) {
	if err := validateTrustedIdentity(identity); err != nil {
		return SaveIntent{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return SaveIntent{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.pending) >= o.capacity {
		return SaveIntent{}, ErrSaveOutboxFull
	}
	if o.nextIntentID == ^uint64(0) {
		return SaveIntent{}, ErrSaveIntentOverflow
	}
	o.nextIntentID++
	intent := SaveIntent{IntentID: o.nextIntentID, Identity: identity, Snapshot: snapshot}
	o.pending = append(o.pending, intent)
	return intent, nil
}

func (o *Outbox) Pending(limit int) []SaveIntent {
	o.mu.Lock()
	defer o.mu.Unlock()
	count := len(o.pending)
	if limit > 0 && limit < count {
		count = limit
	}
	out := make([]SaveIntent, count)
	copy(out, o.pending[:count])
	return out
}

func (o *Outbox) Confirm(intentID uint64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.pending) == 0 {
		return ErrUnknownSaveIntent
	}
	if o.pending[0].IntentID != intentID {
		for _, intent := range o.pending {
			if intent.IntentID == intentID {
				return fmt.Errorf("%w: got=%d want=%d", ErrSaveConfirmOutOfOrder, intentID, o.pending[0].IntentID)
			}
		}
		return ErrUnknownSaveIntent
	}
	copy(o.pending, o.pending[1:])
	o.pending = o.pending[:len(o.pending)-1]
	return nil
}
