package worldruntime

import "github.com/li41/astrahold-server/internal/replication"

// ConvergenceSnapshot 是 Load Lab 在 world owner goroutine 上取得的 bootstrap 收斂快照。
// 它不改變 gameplay semantics，也不應由其他 goroutine 直接讀取 Runtime mutable state。
type ConvergenceSnapshot struct {
	ReplicationViews        int `json:"replication_views"`
	BuiltViews              int `json:"built_views"`
	DesiredRelationships    int `json:"desired_relationships"`
	KnownDesired            int `json:"known_desired"`
	PendingSpawns           int `json:"pending_spawns"`
	PendingDespawns         int `json:"pending_despawns"`
	PendingVitalsSessions   int `json:"pending_vitals_sessions"`
	PendingVitalsEntities   int `json:"pending_vitals_entities"`
	DirtyVitalsEntities     int `json:"dirty_vitals_entities"`
	PendingDynamicSessions  int `json:"pending_dynamic_sessions"`
}

// Converged 只描述 Server application-side lifecycle state；transport Reliable backlog
// 由 tcpudp.Server 另外檢查，兩者同時為零才可開始 steady-state measurement。
func (s ConvergenceSnapshot) Converged(expectedSessions int) bool {
	return expectedSessions > 0 &&
		s.ReplicationViews == expectedSessions &&
		s.BuiltViews == expectedSessions &&
		s.DesiredRelationships == s.KnownDesired &&
		s.PendingSpawns == 0 &&
		s.PendingDespawns == 0 &&
		s.PendingVitalsSessions == 0 &&
		s.PendingVitalsEntities == 0 &&
		s.DirtyVitalsEntities == 0 &&
		s.PendingDynamicSessions == 0
}

// ConvergenceSnapshot 必須由 world owner goroutine 呼叫，例如 Loop.RunObserved callback。
// S3-E.5 只在 all-ready 到 convergence gate 期間啟用這個完整 scan，steady-state 不付此成本。
func (r *Runtime) ConvergenceSnapshot() ConvergenceSnapshot {
	stats := r.replication.ConvergenceStats()
	pendingVitalsEntities := 0
	for _, pending := range r.sessionVitalsPending {
		pendingVitalsEntities += len(pending)
	}

	pendingDynamic := 0
	if r.dynamic != nil && r.dynamicRevision != 0 {
		delivered := 0
		for _, revision := range r.sessionDynamicRevision {
			if revision >= r.dynamicRevision {
				delivered++
			}
		}
		if delivered < stats.Views {
			pendingDynamic = stats.Views - delivered
		}
	}

	return convergenceSnapshotFromReplication(stats, len(r.sessionVitalsPending), pendingVitalsEntities, len(r.dirtyVitalsEntities), pendingDynamic)
}

func convergenceSnapshotFromReplication(stats replication.ConvergenceStats, pendingVitalsSessions, pendingVitalsEntities, dirtyVitalsEntities, pendingDynamicSessions int) ConvergenceSnapshot {
	return ConvergenceSnapshot{
		ReplicationViews:       stats.Views,
		BuiltViews:             stats.BuiltViews,
		DesiredRelationships:   stats.DesiredRelationships,
		KnownDesired:           stats.KnownDesired,
		PendingSpawns:          stats.PendingSpawns,
		PendingDespawns:        stats.PendingDespawns,
		PendingVitalsSessions:  pendingVitalsSessions,
		PendingVitalsEntities:  pendingVitalsEntities,
		DirtyVitalsEntities:    dirtyVitalsEntities,
		PendingDynamicSessions: pendingDynamicSessions,
	}
}
