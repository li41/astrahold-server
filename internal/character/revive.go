package character

import (
	"errors"

	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterNotDefeated = errors.New("character: character is not defeated")
	ErrInvalidRevivePercent = errors.New("character: invalid revive hp percent")
)

// RevivePercent 將已 Defeated 的角色恢復為 MaxHP 的指定百分比。
// percentage 使用整數避免 float rounding drift；結果採 ceil，確保合法百分比至少恢復 1 HP。
//
// 這裡只擁有 Character vitals transition；position、movement input、respawn pending 與
// replication ordering 由 worldruntime 在單一 world-owner command phase協調。
func (s *Service) RevivePercent(id world.EntityID, percent uint8) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrCharacterNotFound
	}
	if percent == 0 || percent > 100 {
		return state, ErrInvalidRevivePercent
	}
	if !state.Defeated {
		return state, ErrCharacterNotDefeated
	}
	hp := (uint64(state.MaxHP)*uint64(percent) + 99) / 100
	if hp == 0 {
		hp = 1
	}
	if hp > uint64(state.MaxHP) {
		hp = uint64(state.MaxHP)
	}
	state.HP = uint32(hp)
	state.Defeated = false
	s.states[id] = state
	return state, nil
}

// ReviveFull 保留 authoritative respawn 的 full-HP transition。
func (s *Service) ReviveFull(id world.EntityID) (State, error) {
	return s.RevivePercent(id, 100)
}
