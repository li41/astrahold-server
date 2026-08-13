package navigation

import (
	"sort"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

// BlockerStates 回傳穩定排序的 runtime blocker snapshot，供 WorldRuntime 做可靠狀態複寫。
// Navigation 仍然不知道 protocol/wire format。
func (n *GameplayNavigator) BlockerStates() []gameplayworld.BlockerState {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ids := make([]string, 0, len(n.blockers))
	for id := range n.blockers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	states := make([]gameplayworld.BlockerState, 0, len(ids))
	for _, id := range ids {
		states = append(states, gameplayworld.BlockerState{ID: id, Enabled: n.enabled[id]})
	}
	return states
}
