package character

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidClassAssignment = errors.New("character: invalid class assignment")
	ErrClassAlreadyAssigned   = errors.New("character: class already assigned")
)

// AssignInitialClass performs the only supported ClassID mutation: unassigned -> one canonical
// class. There is intentionally no general SetClassID or transfer path.
func (s *Service) AssignInitialClass(id world.EntityID, target classid.ID) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if target == "" || !classid.IsCanonical(target) {
		return state, ErrInvalidClassAssignment
	}
	if state.ClassID != "" {
		return state, ErrClassAlreadyAssigned
	}
	state.ClassID = target
	s.states[id] = state
	return state, nil
}
