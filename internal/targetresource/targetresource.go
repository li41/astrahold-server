// Package targetresource owns generic Server-side source-target combat resource state.
package targetresource

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

type ID string

const Flaw ID = "flaw"

var ErrInvalidState = errors.New("targetresource: invalid state")

type Key struct {
	SourceEntityID world.EntityID
	TargetEntityID world.EntityID
	ResourceID     ID
}

type State struct {
	Key
	Current   uint32
	Max       uint32
	ReadyTick uint64
}

type Store struct{ states map[Key]State }

func NewStore() *Store { return &Store{states: make(map[Key]State)} }

func (s *Store) State(key Key) (State, bool) {
	if s == nil {
		return State{}, false
	}
	state, ok := s.states[key]
	return state, ok
}

// TryGain mutates one source-target resource only when its Server-owned internal cooldown is ready.
func (s *Store) TryGain(key Key, amount, max uint32, tick, nextReadyTick uint64) (State, bool, error) {
	if s == nil || key.SourceEntityID == 0 || key.TargetEntityID == 0 || key.SourceEntityID == key.TargetEntityID || key.ResourceID == "" || amount == 0 || max == 0 || nextReadyTick < tick {
		return State{}, false, ErrInvalidState
	}
	state, exists := s.states[key]
	if exists && state.Max != max {
		return state, false, ErrInvalidState
	}
	if !exists {
		state = State{Key: key, Max: max}
	}
	if state.Current >= state.Max || tick < state.ReadyTick {
		return state, false, nil
	}
	missing := state.Max - state.Current
	if amount > missing {
		amount = missing
	}
	state.Current += amount
	state.ReadyTick = nextReadyTick
	s.states[key] = state
	return state, true, nil
}

// ClearEntity removes every resource where entity is either source or target and returns the removed
// states in deterministic order so the runtime can publish authoritative clears to surviving owners.
func (s *Store) ClearEntity(entity world.EntityID) []State {
	if s == nil || entity == 0 {
		return nil
	}
	removed := make([]State, 0)
	for key, state := range s.states {
		if key.SourceEntityID == entity || key.TargetEntityID == entity {
			removed = append(removed, state)
			delete(s.states, key)
		}
	}
	sort.Slice(removed, func(i, j int) bool {
		if removed[i].SourceEntityID != removed[j].SourceEntityID {
			return removed[i].SourceEntityID < removed[j].SourceEntityID
		}
		if removed[i].TargetEntityID != removed[j].TargetEntityID {
			return removed[i].TargetEntityID < removed[j].TargetEntityID
		}
		return removed[i].ResourceID < removed[j].ResourceID
	})
	return removed
}
