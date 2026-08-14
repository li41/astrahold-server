package replication

import (
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
)

// NeedsLifecycleWork 回報本 Session 對目前 shared frame / AOI view 是否仍有
// Spawn / Despawn lifecycle state 需要由 lifecycle-first builder 處理。
//
// 這只作為 Runtime 的 startup work scheduling hint；lifecycle truth 仍完全由
// desired/known + ConfirmSpawn/ConfirmDespawn 決定。若 desired membership 尚未同步到
// dense tracks，也一律回 true，讓正式 builder 先更新 state，而不是錯把 stale view
// 當成已完成 bootstrap。
func (s *Service) NeedsLifecycleWork(sessionID session.ID, frame *simulation.ReplicationFrame, visibleIndices []int) bool {
	state := s.ensureView(sessionID)
	if !sameDesiredIDs(state.desiredIDs, frame, visibleIndices) {
		return true
	}
	if firstUnknownDesired(state) >= 0 {
		return true
	}
	return len(state.departed) > 0
}
