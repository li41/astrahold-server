// Package warehouse owns generic Server-authoritative stack-item storage state.
package warehouse

import (
	"errors"
	"math"
	"sort"
	"strings"
)

var (
	ErrInvalidItemID       = errors.New("warehouse: invalid item archetype id")
	ErrInvalidQuantity     = errors.New("warehouse: invalid quantity")
	ErrInsufficientQuantity = errors.New("warehouse: insufficient quantity")
	ErrQuantityOverflow    = errors.New("warehouse: quantity overflow")
)

type Stack struct {
	ItemArchetypeID string
	Quantity        uint32
}

type Storage struct {
	items map[string]uint32
}

func New() *Storage { return &Storage{items: make(map[string]uint32)} }

func Restore(stacks []Stack) (*Storage, error) {
	storage := New()
	for _, stack := range stacks {
		id := strings.TrimSpace(stack.ItemArchetypeID)
		if id == "" || id != stack.ItemArchetypeID {
			return nil, ErrInvalidItemID
		}
		if stack.Quantity == 0 {
			return nil, ErrInvalidQuantity
		}
		if _, exists := storage.items[id]; exists {
			return nil, ErrInvalidQuantity
		}
		storage.items[id] = stack.Quantity
	}
	return storage, nil
}

func (s *Storage) Quantity(itemArchetypeID string) uint32 {
	if s == nil {
		return 0
	}
	return s.items[itemArchetypeID]
}

func (s *Storage) Add(itemArchetypeID string, quantity uint32) error {
	if s == nil {
		return ErrInvalidQuantity
	}
	id := strings.TrimSpace(itemArchetypeID)
	if id == "" || id != itemArchetypeID {
		return ErrInvalidItemID
	}
	if quantity == 0 {
		return ErrInvalidQuantity
	}
	current := s.items[id]
	if uint64(current)+uint64(quantity) > math.MaxUint32 {
		return ErrQuantityOverflow
	}
	s.items[id] = current + quantity
	return nil
}

func (s *Storage) Remove(itemArchetypeID string, quantity uint32) error {
	if s == nil || quantity == 0 {
		return ErrInvalidQuantity
	}
	current := s.items[itemArchetypeID]
	if current < quantity {
		return ErrInsufficientQuantity
	}
	remaining := current - quantity
	if remaining == 0 {
		delete(s.items, itemArchetypeID)
	} else {
		s.items[itemArchetypeID] = remaining
	}
	return nil
}

func (s *Storage) Snapshot() []Stack {
	if s == nil || len(s.items) == 0 {
		return []Stack{}
	}
	ids := make([]string, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Stack, 0, len(ids))
	for _, id := range ids {
		out = append(out, Stack{ItemArchetypeID: id, Quantity: s.items[id]})
	}
	return out
}
