package character

import "github.com/li41/astrahold-server/internal/world"

// RestoreAliveFull restores an existing, alive combatant to its authored maximum HP/MP.
// It intentionally refuses defeated state so encounter reset cannot bypass the authoritative
// death/respawn lifecycle.
func (s *Service) RestoreAliveFull(id world.EntityID) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if state.Defeated {
		return state, ErrCharacterDefeated
	}
	state.HP = state.MaxHP
	state.MP = state.MaxMP
	s.states[id] = state
	return state, nil
}
