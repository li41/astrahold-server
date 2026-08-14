// Package deathoutcome 定義 Server-owned death outcome event 與 bounded in-memory outbox。
package deathoutcome

import (
	"errors"
	"fmt"
	"sync"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidCapacity   = errors.New("deathoutcome: invalid outbox capacity")
	ErrInvalidEvent      = errors.New("deathoutcome: invalid event")
	ErrOutboxFull        = errors.New("deathoutcome: outbox full")
	ErrRevisionRegression = errors.New("deathoutcome: defeat revision regression")
	ErrOutcomeConflict   = errors.New("deathoutcome: conflicting outcome for defeat revision")
	ErrEventIDOverflow   = errors.New("deathoutcome: event id overflow")
	ErrUnknownEvent      = errors.New("deathoutcome: unknown event")
	ErrConfirmOutOfOrder = errors.New("deathoutcome: confirm out of order")
)

type RespawnBinding struct {
	Scheduled    bool
	SpawnPointID string
	SpawnClass   respawnpolicy.SpawnClass
	Position     world.Position
	DueTick      uint64
}

type Event struct {
	EventID                    uint64
	EntityID                   world.EntityID
	DefeatRevision             uint64
	Context                    respawnpolicy.DeathContext
	DefeatedTick               uint64
	RespawnPolicyRevision      string
	DeathPenaltyPolicyRevision string
	Respawn                    RespawnBinding
	PenaltyTransactionApplied  bool
	CheckpointForfeited        bool
}

// Outbox 是 process-local、thread-safe 的 bounded outbox。World owner只呼叫 Enqueue；
// external consumer可在其他 goroutine Pending/Confirm，因此不需要把 DB / network I/O 放進 world tick。
// 它不是 durable storage；process restart 後內容不保留。
type Outbox struct {
	mu             sync.Mutex
	capacity       int
	nextEventID    uint64
	pending        []Event
	lastByEntity   map[world.EntityID]Event
}

func NewOutbox(capacity int) (*Outbox, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	return &Outbox{
		capacity:     capacity,
		pending:      make([]Event, 0, capacity),
		lastByEntity: make(map[world.EntityID]Event),
	}, nil
}

func (o *Outbox) Capacity() int { return o.capacity }

func (o *Outbox) Depth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pending)
}

// Enqueue 以 (EntityID, DefeatRevision) 對目前 entity incarnation 做 idempotency。
// same revision + same payload 回傳既有 event 且 created=false；same revision不同 payload視為 conflict。
// ResetEntity 會在 leave_world 清 incarnation dedupe，但不會刪除尚未被 consumer confirm 的舊事件。
func (o *Outbox) Enqueue(event Event) (stored Event, created bool, err error) {
	if err := validateEvent(event); err != nil {
		return Event{}, false, err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if last, ok := o.lastByEntity[event.EntityID]; ok {
		if event.DefeatRevision < last.DefeatRevision {
			return Event{}, false, fmt.Errorf("%w: entity=%d got=%d last=%d", ErrRevisionRegression, event.EntityID, event.DefeatRevision, last.DefeatRevision)
		}
		if event.DefeatRevision == last.DefeatRevision {
			candidate := event
			candidate.EventID = last.EventID
			if candidate != last {
				return Event{}, false, fmt.Errorf("%w: entity=%d revision=%d", ErrOutcomeConflict, event.EntityID, event.DefeatRevision)
			}
			return last, false, nil
		}
	}
	if len(o.pending) >= o.capacity {
		return Event{}, false, ErrOutboxFull
	}
	if o.nextEventID == ^uint64(0) {
		return Event{}, false, ErrEventIDOverflow
	}
	o.nextEventID++
	event.EventID = o.nextEventID
	o.pending = append(o.pending, event)
	o.lastByEntity[event.EntityID] = event
	return event, true, nil
}

// Pending 回傳 oldest-first snapshot，不前進 delivery truth。limit<=0代表目前全部 pending。
func (o *Outbox) Pending(limit int) []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	count := len(o.pending)
	if limit > 0 && limit < count {
		count = limit
	}
	out := make([]Event, count)
	copy(out, o.pending[:count])
	return out
}

// Confirm 只允許 oldest-first確認，確保 consumer crash/retry 時不會跳過前面的 event。
func (o *Outbox) Confirm(eventID uint64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.pending) == 0 {
		return ErrUnknownEvent
	}
	if o.pending[0].EventID != eventID {
		for _, event := range o.pending {
			if event.EventID == eventID {
				return fmt.Errorf("%w: got=%d want=%d", ErrConfirmOutOfOrder, eventID, o.pending[0].EventID)
			}
		}
		return ErrUnknownEvent
	}
	copy(o.pending, o.pending[1:])
	o.pending = o.pending[:len(o.pending)-1]
	return nil
}

// ResetEntity 結束目前 EntityID incarnation 的 defeat-revision dedupe；pending event仍保留給 consumer。
func (o *Outbox) ResetEntity(entityID world.EntityID) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.lastByEntity, entityID)
}

func validateEvent(event Event) error {
	if event.EventID != 0 || event.EntityID == 0 || event.DefeatRevision == 0 || !validContext(event.Context) {
		return ErrInvalidEvent
	}
	if event.CheckpointForfeited && !event.PenaltyTransactionApplied {
		return fmt.Errorf("%w: checkpoint forfeiture requires applied penalty transaction", ErrInvalidEvent)
	}
	if event.Respawn.Scheduled {
		if event.Respawn.SpawnPointID == "" || !validSpawnClass(event.Respawn.SpawnClass) || event.Respawn.DueTick <= event.DefeatedTick {
			return fmt.Errorf("%w: invalid respawn binding", ErrInvalidEvent)
		}
	} else if event.Respawn.SpawnPointID != "" || event.Respawn.SpawnClass != "" || event.Respawn.Position != (world.Position{}) || event.Respawn.DueTick != 0 {
		return fmt.Errorf("%w: unscheduled respawn must not carry binding fields", ErrInvalidEvent)
	}
	return nil
}

func validContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege:
		return true
	default:
		return false
	}
}

func validSpawnClass(class respawnpolicy.SpawnClass) bool {
	switch class {
	case respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint, respawnpolicy.SpawnClassSiege:
		return true
	default:
		return false
	}
}
