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
	ErrSaveCompletionFull    = errors.New("characterstate: save completion lane full")
	ErrSaveIntentOverflow    = errors.New("characterstate: save intent id overflow")
	ErrUnknownSaveIntent     = errors.New("characterstate: unknown save intent")
	ErrSaveConfirmOutOfOrder = errors.New("characterstate: save confirm out of order")
)

type SaveIntent struct {
	IntentID             uint64
	Identity             characteridentity.Binding
	Snapshot             Snapshot
	CompletionRequested  bool
}

type Outbox struct {
	mu                    sync.Mutex
	capacity              int
	nextIntentID          uint64
	pending               []SaveIntent
	completed             []SaveIntent
	completionOutstanding int
}

func NewOutbox(capacity int) (*Outbox, error) {
	if capacity <= 0 {
		return nil, ErrInvalidOutboxCapacity
	}
	return &Outbox{
		capacity:  capacity,
		pending:   make([]SaveIntent, 0, capacity),
		completed: make([]SaveIntent, 0, capacity),
	}, nil
}

func (o *Outbox) Capacity() int { return o.capacity }

func (o *Outbox) Depth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pending)
}

func (o *Outbox) CompletionDepth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.completed)
}

func (o *Outbox) Enqueue(identity characteridentity.Binding, snapshot Snapshot) (SaveIntent, error) {
	return o.enqueue(identity, snapshot, false)
}

// EnqueueWithCompletion reserves a process-local completion slot together with the save
// intent. The durable worker may publish that completion only after Store application and
// checkpoint advancement. CompletionRequested is deliberately not part of the durable wire
// schema: after a process crash there is no live runtime mutation waiting for an acknowledgement;
// reconnect restores the already-durable Snapshot instead.
func (o *Outbox) EnqueueWithCompletion(identity characteridentity.Binding, snapshot Snapshot) (SaveIntent, error) {
	return o.enqueue(identity, snapshot, true)
}

func (o *Outbox) enqueue(identity characteridentity.Binding, snapshot Snapshot, completionRequested bool) (SaveIntent, error) {
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
	if completionRequested && o.completionOutstanding >= o.capacity {
		return SaveIntent{}, ErrSaveCompletionFull
	}
	if o.nextIntentID == ^uint64(0) {
		return SaveIntent{}, ErrSaveIntentOverflow
	}
	o.nextIntentID++
	intent := SaveIntent{
		IntentID:            o.nextIntentID,
		Identity:            identity,
		Snapshot:            snapshot,
		CompletionRequested: completionRequested,
	}
	o.pending = append(o.pending, intent)
	if completionRequested {
		o.completionOutstanding++
	}
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

// Complete publishes an already-durable intent to the world-owner completion lane. A slot was
// reserved atomically with EnqueueWithCompletion, so this operation cannot fail for a valid
// completion-requested intent. Callers must invoke it only after the durable checkpoint advances.
func (o *Outbox) Complete(intent SaveIntent) {
	if !intent.CompletionRequested {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.completed = append(o.completed, intent)
}

// TakeCompleted transfers immutable durability acknowledgements to the world owner. Removing
// an acknowledgement releases the completion reservation made by EnqueueWithCompletion.
func (o *Outbox) TakeCompleted(limit int) []SaveIntent {
	o.mu.Lock()
	defer o.mu.Unlock()
	count := len(o.completed)
	if limit > 0 && limit < count {
		count = limit
	}
	if count == 0 {
		return nil
	}
	out := make([]SaveIntent, count)
	copy(out, o.completed[:count])
	copy(o.completed, o.completed[count:])
	o.completed = o.completed[:len(o.completed)-count]
	o.completionOutstanding -= count
	return out
}
