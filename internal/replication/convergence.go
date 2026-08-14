package replication

// ConvergenceStats 描述目前所有 replication view 的 lifecycle 收斂狀態。
// 這是診斷 / Load Lab 用的 owner-thread snapshot；不應在 production hot path 每 tick 掃描。
type ConvergenceStats struct {
	Views              int
	BuiltViews         int
	DesiredRelationships int
	KnownDesired       int
	PendingSpawns      int
	PendingDespawns    int
}

// ConvergenceStats 只可由擁有 replication.Service 的 world owner goroutine 呼叫。
// 它刻意走完整 view scan，讓 Load Lab 可以用真實 desired/known lifecycle state 建立 semantic gate。
func (s *Service) ConvergenceStats() ConvergenceStats {
	var stats ConvergenceStats
	if s == nil {
		return stats
	}
	stats.Views = len(s.views)
	for _, state := range s.views {
		if state == nil {
			continue
		}
		if state.buildNumber > 0 {
			stats.BuiltViews++
		}
		stats.DesiredRelationships += len(state.desiredIDs)
		for i := range state.desiredIDs {
			if i < len(state.tracks) && state.tracks[i].known {
				stats.KnownDesired++
			} else {
				stats.PendingSpawns++
			}
		}
		for id := range state.known {
			if !containsDesiredID(state.desiredIDs, id) {
				stats.PendingDespawns++
			}
		}
	}
	return stats
}
