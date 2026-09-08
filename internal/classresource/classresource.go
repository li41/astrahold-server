// Package classresource defines stable Server-side class combat resource identities and runtime state.
package classresource

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/world"
)

type ID string

const (
	Empty   ID = ""
	Resolve ID = "resolve"
)

var (
	ErrStateNotFound      = errors.New("classresource: state not found")
	ErrResourceMismatch  = errors.New("classresource: resource mismatch")
)

type Definition struct {
	ID  ID
	Max uint32
}

type State struct {
	EntityID  world.EntityID
	ResourceID ID
	Current   uint32
	Max       uint32
}

type Service struct {
	states map[world.EntityID]State
}

func NewService() *Service {
	return &Service{states: make(map[world.EntityID]State)}
}

// PrimaryForClass returns the authored primary combat resource for a class. A missing definition
// means that class has not yet shipped an authoritative class-resource contract.
func PrimaryForClass(id classid.ID) (Definition, bool) {
	switch id {
	case classid.Oathguard:
		return Definition{ID: Resolve, Max: 100}, true
	default:
		return Definition{}, false
	}
}

// ResetForClass installs a fresh combat-incarnation resource state. It intentionally starts at zero
// and is not a persistence API: reconnect/respawn policy may later choose different initialization.
func (s *Service) ResetForClass(entityID world.EntityID, classID classid.ID) (State, bool) {
	definition, ok := PrimaryForClass(classID)
	if !ok || entityID == 0 {
		delete(s.states, entityID)
		return State{}, false
	}
	state := State{EntityID: entityID, ResourceID: definition.ID, Max: definition.Max}
	s.states[entityID] = state
	return state, true
}

func (s *Service) Remove(entityID world.EntityID) {
	delete(s.states, entityID)
}

func (s *Service) State(entityID world.EntityID) (State, bool) {
	state, ok := s.states[entityID]
	return state, ok
}

// Gain applies one world-owner-authoritative resource gain and clamps at the authored maximum.
func (s *Service) Gain(entityID world.EntityID, resourceID ID, amount uint32) (State, error) {
	state, ok := s.states[entityID]
	if !ok {
		return State{}, ErrStateNotFound
	}
	if resourceID == Empty || state.ResourceID != resourceID {
		return state, ErrResourceMismatch
	}
	if amount == 0 || state.Current >= state.Max {
		return state, nil
	}
	missing := state.Max - state.Current
	if amount > missing {
		amount = missing
	}
	state.Current += amount
	s.states[entityID] = state
	return state, nil
}
