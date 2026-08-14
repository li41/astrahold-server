package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestNeedsLifecycleWorkTracksCurrentMembershipWithoutTreatingDirtyTransformAsLifecycle(t *testing.T) {
	svc := NewService()
	sid := session.ID(91)
	svc.Register(sid)
	limits := LifecycleLimits{MaxSpawns: 8, MaxDespawns: 8, MaxMessages: 8}

	frame, visible := lifecycleFrame(1, 2)
	if !svc.NeedsLifecycleWork(sid, frame, visible) {
		t.Fatal("fresh view should require lifecycle work")
	}
	batch := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	for _, id := range lifecycleIDs(batch, true) {
		svc.ConfirmSpawn(sid, id)
	}
	if svc.NeedsLifecycleWork(sid, frame, visible) {
		t.Fatal("fully known unchanged membership should not require lifecycle work")
	}

	// Dirty transform 是 realtime scheduler 的工作，不應重新變成 lifecycle pending。
	frame.Entities[1].Transform.Position.Z += 1
	frame.TransformGenerations[1]++
	if svc.NeedsLifecycleWork(sid, frame, visible) {
		t.Fatal("dirty transform was misclassified as lifecycle work")
	}

	// 新 desired membership 尚未同步 dense tracks 時必須先走正式 lifecycle builder。
	joined, joinedVisible := lifecycleFrameIDs(2, []world.EntityID{1, 2, 3})
	if !svc.NeedsLifecycleWork(sid, joined, joinedVisible) {
		t.Fatal("appended desired entity should require lifecycle work")
	}
	joinedBatch := svc.BuildFrameLifecycleFirst(sid, 1, 0, joined, joinedVisible, limits)
	for _, id := range lifecycleIDs(joinedBatch, true) {
		svc.ConfirmSpawn(sid, id)
	}
	if svc.NeedsLifecycleWork(sid, joined, joinedVisible) {
		t.Fatal("confirmed appended membership should be lifecycle-complete")
	}
}
