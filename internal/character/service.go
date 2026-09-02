// Package character 定義角色生命狀態、resource state 與 damage target ownership。
package character

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

const DefaultMaxMP uint32 = 100

var (
	ErrInvalidMaxHP        = errors.New("character: invalid max hp")
	ErrInvalidMaxMP        = errors.New("character: invalid max mp")
	ErrInvalidState        = errors.New("character: invalid state")
	ErrCharacterExists     = errors.New("character: character already exists")
	ErrCharacterNotFound   = errors.New("character: character not found")
	ErrCharacterDefeated   = errors.New("character: character defeated")
	ErrInsufficientResource = errors.New("character: insufficient resource")
)

type State struct {
	EntityID world.EntityID
	HP       uint32
	MaxHP    uint32
	MP       uint32
	MaxMP    uint32
	Defeated bool
}

type Service struct {
	defaultMaxHP uint32
	defaultMaxMP uint32
	states       map[world.EntityID]State
}

// NewService remains for focused character tests and legacy callers. Runtime code should use
// NewServiceWithResources so gameplay defaults are explicit in worldruntime.Config.
func NewService(defaultMaxHP uint32) (*Service, error) {
	return NewServiceWithResources(defaultMaxHP, DefaultMaxMP)
}

func NewServiceWithResources(defaultMaxHP, defaultMaxMP uint32) (*Service, error) {
	if defaultMaxHP == 0 {
		return nil, ErrInvalidMaxHP
	}
	if defaultMaxMP == 0 {
		return nil, ErrInvalidMaxMP
	}
	return &Service{defaultMaxHP: defaultMaxHP, defaultMaxMP: defaultMaxMP, states: make(map[world.EntityID]State)}, nil
}

func (s *Service) Register(id world.EntityID) error {
	return s.RegisterState(State{EntityID: id, HP: s.defaultMaxHP, MaxHP: s.defaultMaxHP, MP: s.defaultMaxMP, MaxMP: s.defaultMaxMP})
}

// RegisterState installs an already-authoritative character vitals state for a newly
// spawned world incarnation. It is intentionally registration-only: callers cannot use
// it to mutate an existing character and bypass normal gameplay transitions.
func (s *Service) RegisterState(state State) error {
	if state.EntityID == 0 {
		return ErrCharacterNotFound
	}
	if _, exists := s.states[state.EntityID]; exists {
		return ErrCharacterExists
	}
	if err := validateState(state); err != nil {
		return err
	}
	s.states[state.EntityID] = state
	return nil
}

func validateState(state State) error {
	if state.MaxHP == 0 || state.HP > state.MaxHP || state.MaxMP == 0 || state.MP > state.MaxMP {
		return ErrInvalidState
	}
	if state.Defeated {
		if state.HP != 0 {
			return ErrInvalidState
		}
	} else if state.HP == 0 {
		return ErrInvalidState
	}
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

// SpendMP performs one world-owner-authoritative resource transition. A rejected spend does
// not mutate state; callers should invoke it only after target/range/LOS legality has passed.
func (s *Service) SpendMP(id world.EntityID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if amount == 0 {
		return state, nil
	}
	if state.MP < amount {
		return state, ErrInsufficientResource
	}
	state.MP -= amount
	s.states[id] = state
	return state, nil
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
