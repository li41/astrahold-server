// Package character 定義角色生命狀態、resource state 與 damage target ownership。
package character

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/targetresource"
	"github.com/li41/astrahold-server/internal/world"
)

const DefaultMaxMP uint32 = 100

var (
	ErrInvalidMaxHP         = errors.New("character: invalid max hp")
	ErrInvalidMaxMP         = errors.New("character: invalid max mp")
	ErrInvalidState         = errors.New("character: invalid state")
	ErrCharacterExists      = errors.New("character: character already exists")
	ErrCharacterNotFound    = errors.New("character: character not found")
	ErrCharacterDefeated    = errors.New("character: character defeated")
	ErrInsufficientResource = errors.New("character: insufficient resource")
	ErrResourceFull         = errors.New("character: resource already full")
)

type State struct {
	EntityID world.EntityID
	HP       uint32
	MaxHP    uint32
	MP       uint32
	MaxMP    uint32

	// ClassResource* names are retained as Protocol v27 compatibility surface. The runtime
	// definitions themselves are classless and owned by internal/actionresource.
	ClassResourceID       actionresource.ID
	ClassResource         uint32
	MaxClassResource      uint32
	ClassResourceProgress uint32
	Defeated              bool
}

type Service struct {
	defaultMaxHP    uint32
	defaultMaxMP    uint32
	states          map[world.EntityID]State
	targetResources *targetresource.Store
}

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
	return &Service{
		defaultMaxHP:    defaultMaxHP,
		defaultMaxMP:    defaultMaxMP,
		states:          make(map[world.EntityID]State),
		targetResources: targetresource.NewStore(),
	}, nil
}

func (s *Service) Register(id world.EntityID) error {
	return s.RegisterState(State{
		EntityID: id,
		HP:       s.defaultMaxHP,
		MaxHP:    s.defaultMaxHP,
		MP:       s.defaultMaxMP,
		MaxMP:    s.defaultMaxMP,
	})
}

func (s *Service) RegisterState(state State) error {
	if state.EntityID == 0 {
		return ErrCharacterNotFound
	}
	if _, exists := s.states[state.EntityID]; exists {
		return ErrCharacterExists
	}
	if state.MP == 0 && state.MaxMP == 0 {
		state.MP = s.defaultMaxMP
		state.MaxMP = s.defaultMaxMP
	}
	if err := normalizeActionResourceState(&state); err != nil {
		return err
	}
	if err := validateState(state); err != nil {
		return err
	}
	s.states[state.EntityID] = state
	return nil
}

func normalizeActionResourceState(state *State) error {
	if state == nil {
		return ErrInvalidState
	}
	if state.ClassResourceID == actionresource.Empty {
		return nil
	}
	definition, ok := actionresource.DefinitionForID(state.ClassResourceID)
	if !ok {
		return ErrInvalidState
	}
	if state.MaxClassResource == 0 {
		if state.ClassResource != 0 || state.ClassResourceProgress != 0 {
			return ErrInvalidState
		}
		state.MaxClassResource = definition.Max
	}
	return nil
}

func validateState(state State) error {
	if state.MaxHP == 0 || state.HP > state.MaxHP || state.MaxMP == 0 || state.MP > state.MaxMP {
		return ErrInvalidState
	}
	if state.ClassResource > state.MaxClassResource {
		return ErrInvalidState
	}
	if state.ClassResourceID == actionresource.Empty {
		if state.ClassResource != 0 || state.MaxClassResource != 0 || state.ClassResourceProgress != 0 {
			return ErrInvalidState
		}
	} else {
		definition, ok := actionresource.DefinitionForID(state.ClassResourceID)
		if !ok || state.MaxClassResource != definition.Max {
			return ErrInvalidState
		}
		if state.ClassResourceProgress > 0 && (definition.ProgressThreshold == 0 || state.ClassResourceProgress >= definition.ProgressThreshold || state.ClassResource >= state.MaxClassResource) {
			return ErrInvalidState
		}
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
	s.targetResources.ClearEntity(id)
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

func (s *Service) GainActionResource(id world.EntityID, resourceID actionresource.ID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if resourceID == actionresource.Empty || state.ClassResourceID != resourceID || state.MaxClassResource == 0 {
		return state, actionresource.ErrResourceMismatch
	}
	if amount == 0 || state.ClassResource >= state.MaxClassResource {
		return state, nil
	}
	missing := state.MaxClassResource - state.ClassResource
	if amount > missing {
		amount = missing
	}
	state.ClassResource += amount
	s.states[id] = state
	return state, nil
}

func (s *Service) SpendActionResource(id world.EntityID, resourceID actionresource.ID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if resourceID == actionresource.Empty || state.ClassResourceID != resourceID || state.MaxClassResource == 0 {
		return state, actionresource.ErrResourceMismatch
	}
	if amount == 0 {
		return state, nil
	}
	if state.ClassResource < amount {
		return state, ErrInsufficientResource
	}
	state.ClassResource -= amount
	s.states[id] = state
	return state, nil
}

func (s *Service) GainActionResourceProgress(id world.EntityID, resourceID actionresource.ID, amount uint32) (State, bool, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, false, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, false, ErrCharacterDefeated
	}
	definition, defined := actionresource.DefinitionForID(state.ClassResourceID)
	if !defined || resourceID == actionresource.Empty || state.ClassResourceID != resourceID || definition.ID != resourceID || definition.Max != state.MaxClassResource || definition.ProgressThreshold == 0 {
		return state, false, actionresource.ErrResourceMismatch
	}
	if amount == 0 || state.ClassResource >= state.MaxClassResource {
		return state, false, nil
	}
	totalProgress := uint64(state.ClassResourceProgress) + uint64(amount)
	threshold := uint64(definition.ProgressThreshold)
	gained := uint32(totalProgress / threshold)
	state.ClassResourceProgress = uint32(totalProgress % threshold)
	visibleChanged := gained > 0
	if gained > 0 {
		missing := state.MaxClassResource - state.ClassResource
		if gained >= missing {
			state.ClassResource = state.MaxClassResource
			state.ClassResourceProgress = 0
		} else {
			state.ClassResource += gained
		}
	}
	s.states[id] = state
	return state, visibleChanged, nil
}

func (s *Service) GainTargetResource(sourceID, targetID world.EntityID, resourceID targetresource.ID, amount, max uint32, tick, nextReadyTick uint64) (targetresource.State, bool, error) {
	source, ok := s.states[sourceID]
	if !ok {
		return targetresource.State{}, false, ErrCharacterNotFound
	}
	if source.Defeated {
		return targetresource.State{}, false, ErrCharacterDefeated
	}
	return s.targetResources.TryGain(targetresource.Key{SourceEntityID: sourceID, TargetEntityID: targetID, ResourceID: resourceID}, amount, max, tick, nextReadyTick)
}

func (s *Service) GainTargetResourcePreservingReadyTick(sourceID, targetID world.EntityID, resourceID targetresource.ID, amount, max uint32) (targetresource.State, bool, error) {
	source, ok := s.states[sourceID]
	if !ok {
		return targetresource.State{}, false, ErrCharacterNotFound
	}
	if source.Defeated {
		return targetresource.State{}, false, ErrCharacterDefeated
	}
	return s.targetResources.GainPreservingReadyTick(targetresource.Key{SourceEntityID: sourceID, TargetEntityID: targetID, ResourceID: resourceID}, amount, max)
}

func (s *Service) SpendTargetResource(sourceID, targetID world.EntityID, resourceID targetresource.ID, amount uint32) (targetresource.State, error) {
	source, ok := s.states[sourceID]
	if !ok {
		return targetresource.State{}, ErrCharacterNotFound
	}
	if source.Defeated {
		return targetresource.State{}, ErrCharacterDefeated
	}
	state, err := s.targetResources.Spend(targetresource.Key{SourceEntityID: sourceID, TargetEntityID: targetID, ResourceID: resourceID}, amount)
	if errors.Is(err, targetresource.ErrInsufficientResource) {
		return state, ErrInsufficientResource
	}
	return state, err
}

func (s *Service) TargetResourceState(sourceID, targetID world.EntityID, resourceID targetresource.ID) (targetresource.State, bool) {
	return s.targetResources.State(targetresource.Key{SourceEntityID: sourceID, TargetEntityID: targetID, ResourceID: resourceID})
}

func (s *Service) ClearTargetResourcesForEntity(entityID world.EntityID) []targetresource.State {
	return s.targetResources.ClearEntity(entityID)
}

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

func (s *Service) RestoreHP(id world.EntityID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if state.HP >= state.MaxHP {
		return state, ErrResourceFull
	}
	missing := state.MaxHP - state.HP
	if amount > missing {
		amount = missing
	}
	state.HP += amount
	s.states[id] = state
	return state, nil
}

func (s *Service) RestoreMP(id world.EntityID, amount uint32) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	if state.MP >= state.MaxMP {
		return state, ErrResourceFull
	}
	missing := state.MaxMP - state.MP
	if amount > missing {
		amount = missing
	}
	state.MP += amount
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
