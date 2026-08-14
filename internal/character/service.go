// Package character 定義角色生命狀態與 damage target ownership。
package character

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidMaxHP   = errors.New("character: invalid max hp")
	ErrCharacterExists = errors.New("character: character already exists")
	ErrCharacterNotFound = errors.New("character: character not found")
	ErrCharacterDefeated = errors.New("character: character defeated")
)

type State struct {
	EntityID world.EntityID
	HP       uint32
	MaxHP    uint32
	Defeated bool
}

type Service struct {
	defaultMaxHP uint32
	states       map[world.EntityID]State
}

func NewService(defaultMaxHP uint32) (*Service, error) {
	if defaultMaxHP == 0 {
		return nil, ErrInvalidMaxHP
	}
	return &Service{defaultMaxHP: defaultMaxHP, states: make(map[world.EntityID]State)}, nil
}

func (s *Service) Register(id world.EntityID) error {
	if id == 0 {
		return ErrCharacterNotFound
	}
	if _, exists := s.states[id]; exists {
		return ErrCharacterExists
	}
	s.states[id] = State{EntityID: id, HP: s.defaultMaxHP, MaxHP: s.defaultMaxHP}
	return nil
}

func (s *Service) Remove(id world.EntityID) {
	delete(s.states, id)
}

func (s *Service) State(id world.EntityID) (State, bool) {
	state, ok := s.states[id]
	return state, ok
}

func (s *Service) States() []State {
	out := make([]State, 0, len(s.states))
	for _, state := range s.states {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityID < out[j].EntityID })
	return out
}

func (s *Service) ApplyDamage(id world.EntityID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if amount >= state.HP {
		state.HP = 0
		state.Defeated = true
	} else {
		state.HP -= amount
	}
	s.states[id] = state
	return state, nil
}
