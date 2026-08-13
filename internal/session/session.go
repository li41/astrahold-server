// Package session 管理連線生命週期與世界 Entity 的綁定；不直接修改 simulation state。
package session

import (
	"errors"
	"sort"
	"sync"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

type ID uint64

var (
	ErrInvalidSession   = errors.New("session: invalid session")
	ErrSessionExists    = errors.New("session: session already exists")
	ErrSessionNotFound  = errors.New("session: session not found")
	ErrConnectionClosed = errors.New("session: connection closed")
	ErrBackpressure     = errors.New("session: outbound queue full")
	ErrStaleInput       = errors.New("session: stale input sequence")
)

type Connection interface {
	TrySend(protocol.Envelope) error
	Close() error
}

// QueueConnection 是 transport-neutral 的非阻塞 outbound 邊界。
// Network writer 之後可分別消費 Reliable / Realtime queue。
type QueueConnection struct {
	reliable  chan protocol.Envelope
	realtime  chan protocol.Envelope
	done      chan struct{}
	closeOnce sync.Once
}

func NewQueueConnection(reliableCapacity, realtimeCapacity int) *QueueConnection {
	if reliableCapacity <= 0 || realtimeCapacity <= 0 {
		panic("session: queue capacity must be > 0")
	}
	return &QueueConnection{reliable: make(chan protocol.Envelope, reliableCapacity), realtime: make(chan protocol.Envelope, realtimeCapacity), done: make(chan struct{})}
}
func (c *QueueConnection) TrySend(e protocol.Envelope) error {
	select {
	case <-c.done:
		return ErrConnectionClosed
	default:
	}
	var target chan protocol.Envelope
	switch e.Delivery {
	case protocol.DeliveryReliableOrdered:
		target = c.reliable
	case protocol.DeliveryRealtimeSequenced:
		target = c.realtime
	default:
		return ErrInvalidSession
	}
	select {
	case target <- e:
		return nil
	case <-c.done:
		return ErrConnectionClosed
	default:
		return ErrBackpressure
	}
}
func (c *QueueConnection) Close() error                       { c.closeOnce.Do(func() { close(c.done) }); return nil }
func (c *QueueConnection) Reliable() <-chan protocol.Envelope { return c.reliable }
func (c *QueueConnection) Realtime() <-chan protocol.Envelope { return c.realtime }
func (c *QueueConnection) Done() <-chan struct{}              { return c.done }

type Session struct {
	ID                         ID
	EntityID                   world.EntityID
	AOIRadius                  float32
	connection                 Connection
	lastProcessedInputSequence uint32
	nextReliableSequence       uint32
	nextRealtimeSequence       uint32
}

func New(id ID, entityID world.EntityID, aoiRadius float32, connection Connection) (*Session, error) {
	if id == 0 || entityID == 0 || aoiRadius <= 0 || connection == nil {
		return nil, ErrInvalidSession
	}
	return &Session{ID: id, EntityID: entityID, AOIRadius: aoiRadius, connection: connection}, nil
}
func (s *Session) Connection() Connection             { return s.connection }
func (s *Session) LastProcessedInputSequence() uint32 { return s.lastProcessedInputSequence }
func (s *Session) ValidateInputSequence(sequence uint32) error {
	if sequence <= s.lastProcessedInputSequence {
		return ErrStaleInput
	}
	return nil
}
func (s *Session) MarkProcessedInput(sequence uint32) {
	if sequence > s.lastProcessedInputSequence {
		s.lastProcessedInputSequence = sequence
	}
}
func (s *Session) NextOutboundSequence(delivery protocol.Delivery) uint32 {
	switch delivery {
	case protocol.DeliveryReliableOrdered:
		s.nextReliableSequence++
		return s.nextReliableSequence
	case protocol.DeliveryRealtimeSequenced:
		s.nextRealtimeSequence++
		return s.nextRealtimeSequence
	default:
		return 0
	}
}

type Registry struct{ sessions map[ID]*Session }

func NewRegistry() *Registry { return &Registry{sessions: make(map[ID]*Session)} }
func (r *Registry) Add(s *Session) error {
	if s == nil {
		return ErrInvalidSession
	}
	if _, ok := r.sessions[s.ID]; ok {
		return ErrSessionExists
	}
	r.sessions[s.ID] = s
	return nil
}
func (r *Registry) Remove(id ID) (*Session, error) {
	s, ok := r.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	delete(r.sessions, id)
	return s, nil
}
func (r *Registry) Get(id ID) (*Session, bool) { s, ok := r.sessions[id]; return s, ok }
func (r *Registry) List() []*Session {
	ids := make([]ID, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*Session, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.sessions[id])
	}
	return out
}
