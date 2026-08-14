package character

import (
	"errors"

	"github.com/li41/astrahold-server/internal/world"
)

var ErrCharacterNotDefeated = errors.New("character: character is not defeated")

// ReviveFull 將已 Defeated 的角色恢復為 full HP。
//
// 這裡只擁有 Character vitals transition；respawn position、movement input 與
// replication ordering 由 worldruntime 在單一 world-owner command phase協調。
func (s *Service) ReviveFull(id world.EntityID) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if !state.Defeated {
		return state, ErrCharacterNotDefeated
	}
	state.HP = state.MaxHP
	state.Defeated = false
	s.states[id] = state
	return state, nil
}
