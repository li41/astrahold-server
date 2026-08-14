package replication

import (
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

// desiredMembershipRemoved 保留給 membership-diff 單元測試與 rare-path判斷工具。
// Production semantic-converged churn 已使用 rebuildPendingDepartedFromDesiredDiff 直接產生 sorted removed IDs。
func desiredMembershipRemoved(previous []world.EntityID, frame *simulation.ReplicationFrame, visibleIndices []int) bool {
	if len(previous) == 0 {
		return false
	}
	previousIndex := 0
	for _, frameIndex := range visibleIndices {
		if frameIndex < 0 || frameIndex >= len(frame.Entities) {
			continue
		}
		id := frame.Entities[frameIndex].ID
		for previousIndex < len(previous) && previous[previousIndex] < id {
			return true
		}
		if previousIndex < len(previous) && previous[previousIndex] == id {
			previousIndex++
			if previousIndex == len(previous) {
				return false
			}
		}
	}
	return previousIndex < len(previous)
}
