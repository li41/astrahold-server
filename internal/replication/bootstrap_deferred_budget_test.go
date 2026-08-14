package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLifecycleDisabledBudgetUpdatesDesiredWithoutMaterializingLifecycle(t *testing.T) {
	svc := NewService()
	sid := session.ID(34)
	svc.Register(sid)
	initial, initialVisible := lifecycleFrameIDs(1, []world.EntityID{1, 2, 3, 4})
	first := svc.BuildFrameLifecycleFirst(sid, 1, 0, initial, initialVisible, LifecycleLimits{})
	for _, id := range lifecycleIDs(first, true) {
		svc.ConfirmSpawn(sid, id)
	}

	changed, changedVisible := lifecycleFrameIDs(2, []world.EntityID{1, 2, 5, 6})
	deferred := svc.BuildFrameLifecycleFirst(sid, 1, 0, changed, changedVisible, LifecycleLimits{MaxMessages: -1})
	if got := lifecycleIDs(deferred, true); len(got) != 0 {
		t.Fatalf("disabled global budget materialized spawns: %v", got)
	}
	if got := lifecycleIDs(deferred, false); len(got) != 0 {
		t.Fatalf("disabled global budget materialized despawns: %v", got)
	}
	stats := svc.ConvergenceStats()
	if stats.DesiredRelationships != 4 || stats.KnownDesired != 2 || stats.PendingSpawns != 2 || stats.PendingDespawns != 2 {
		t.Fatalf("desired state not updated under deferred budget: %+v", stats)
	}
}
