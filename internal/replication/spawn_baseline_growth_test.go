package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLifecycleDesiredGrowthPreservesConfirmedSpawnTransformBaseline(t *testing.T) {
	svc := NewService()
	sid := session.ID(61)
	svc.Register(sid)
	limits := LifecycleLimits{MaxSpawns: 2, MaxDespawns: 2, MaxMessages: 2}

	firstFrame, firstVisible := lifecycleFrameIDs(10, []world.EntityID{1, 2})
	first := svc.BuildFrameLifecycleFirst(sid, 1, 0, firstFrame, firstVisible, limits)
	if got := lifecycleIDs(first, true); !equalIDs(got, []world.EntityID{1, 2}) {
		t.Fatalf("first spawn ids=%v want=[1 2]", got)
	}
	for _, id := range []world.EntityID{1, 2} {
		svc.ConfirmSpawn(sid, id)
	}

	// Mass join 期間 desired 只成長。rebuildDesiredTracks 不應把先前 Spawn 已攜帶的
	// authoritative transform baseline 降回 generation=0。
	grownFrame, grownVisible := lifecycleFrameIDs(20, []world.EntityID{1, 2, 3})
	grown := svc.BuildFrameLifecycleFirst(sid, 1, 0, grownFrame, grownVisible, limits)
	if got := lifecycleIDs(grown, true); !equalIDs(got, []world.EntityID{3}) {
		t.Fatalf("growth spawn ids=%v want=[3]", got)
	}
	if got := snapshotEntityIDs(grown); len(got) != 0 {
		t.Fatalf("bootstrap growth emitted remote snapshot before final Spawn confirm: %v", got)
	}
	svc.ConfirmSpawn(sid, 3)

	converged := svc.BuildFrameLifecycleFirst(sid, 1, 0, grownFrame, grownVisible, limits)
	if got := snapshotEntityIDs(converged); len(got) != 0 {
		t.Fatalf("unchanged confirmed Spawn transforms became dirty after desired growth: %v", got)
	}
}
