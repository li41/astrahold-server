package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLifecycleSpawnBudgetMakesBoundedProgress(t *testing.T) {
	svc := NewService()
	sid := session.ID(31)
	svc.Register(sid)
	frame, visible := lifecycleFrame(10, 5)
	limits := LifecycleLimits{MaxSpawns: 2, MaxDespawns: 2}

	first := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(first, true); !equalIDs(got, []world.EntityID{1, 2}) {
		t.Fatalf("first spawn ids=%v want=[1 2]", got)
	}
	if first.Stats.SpawnCandidates != 3 || first.Stats.SpawnSelected != 2 || first.Stats.SpawnDeferred != 1 {
		t.Fatalf("first bounded scan stats=%+v", first.Stats)
	}
	if got := snapshotEntityIDs(first); len(got) != 0 {
		t.Fatalf("unknown entities entered snapshot: %v", got)
	}
	for _, id := range []world.EntityID{1, 2} {
		svc.ConfirmSpawn(sid, id)
	}

	second := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := snapshotEntityIDs(second); len(got) != 0 {
		t.Fatalf("bootstrap view emitted remote snapshot before all spawns: %v", got)
	}
	if got := lifecycleIDs(second, true); !equalIDs(got, []world.EntityID{3, 4}) {
		t.Fatalf("second spawn ids=%v want=[3 4]", got)
	}
	if second.Stats.SpawnCandidates != 3 || second.Stats.SpawnSelected != 2 || second.Stats.SpawnDeferred != 1 {
		t.Fatalf("second bounded scan stats=%+v", second.Stats)
	}
	for _, id := range []world.EntityID{3, 4} {
		svc.ConfirmSpawn(sid, id)
	}

	third := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(third, true); !equalIDs(got, []world.EntityID{5}) {
		t.Fatalf("third spawn ids=%v want=[5]", got)
	}
	if third.Stats.SpawnCandidates != 1 || third.Stats.SpawnSelected != 1 || third.Stats.SpawnDeferred != 0 {
		t.Fatalf("third spawn stats=%+v", third.Stats)
	}
	svc.ConfirmSpawn(sid, 5)

	// Reliable Spawn 已帶同一份 authoritative transform，因此 unchanged Entity 不應在
	// all-known 後立刻再製造一份 realtime snapshot candidate。
	fourth := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := snapshotEntityIDs(fourth); len(got) != 0 {
		t.Fatalf("unchanged spawn baseline repeated in realtime snapshot: %v", got)
	}

	// 真正的 transform generation 改變後，既有 dirty/cadence scheduler 必須正常恢復。
	frame.Entities[1].Transform.Position.Z += 1
	frame.TransformGenerations[1]++
	fifth := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := snapshotEntityIDs(fifth); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("dirty entity after spawn baseline ids=%v want=[2]", got)
	}
}

func TestLifecycleDespawnBudgetMakesBoundedProgress(t *testing.T) {
	svc := NewService()
	sid := session.ID(32)
	svc.Register(sid)
	all := make([]world.EntityState, 5)
	for i := range all {
		all[i] = world.EntityState{ID: world.EntityID(i + 1), Kind: world.EntityPlayer}
	}
	primeKnown(svc, sid, 1, all)
	limits := LifecycleLimits{MaxSpawns: 2, MaxDespawns: 2}

	frame, visible := lifecycleFrame(20, 1)
	first := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(first, false); !equalIDs(got, []world.EntityID{2, 3}) {
		t.Fatalf("first despawn ids=%v want=[2 3]", got)
	}
	if first.Stats.DespawnCandidates != 4 || first.Stats.DespawnSelected != 2 || first.Stats.DespawnDeferred != 2 {
		t.Fatalf("first despawn stats=%+v", first.Stats)
	}
	for _, id := range []world.EntityID{2, 3} {
		svc.ConfirmDespawn(sid, id)
	}

	second := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(second, false); !equalIDs(got, []world.EntityID{4, 5}) {
		t.Fatalf("second despawn ids=%v want=[4 5]", got)
	}
	if second.Stats.DespawnCandidates != 2 || second.Stats.DespawnSelected != 2 || second.Stats.DespawnDeferred != 0 {
		t.Fatalf("second despawn stats=%+v", second.Stats)
	}
}

func TestLifecycleCombinedBudgetPrioritizesDepartedBeforeUnknown(t *testing.T) {
	svc := NewService()
	sid := session.ID(33)
	svc.Register(sid)
	initial := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer},
		{ID: 2, Kind: world.EntityPlayer},
		{ID: 3, Kind: world.EntityPlayer},
		{ID: 4, Kind: world.EntityPlayer},
	}
	primeKnown(svc, sid, 1, initial)

	frame, visible := lifecycleFrameIDs(30, []world.EntityID{1, 2, 5, 6})
	limits := LifecycleLimits{MaxSpawns: 2, MaxDespawns: 2, MaxMessages: 2}
	first := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(first, false); !equalIDs(got, []world.EntityID{3, 4}) {
		t.Fatalf("first churn despawns=%v want=[3 4]", got)
	}
	if got := lifecycleIDs(first, true); len(got) != 0 {
		t.Fatalf("combined budget should be consumed by stale despawns first, spawns=%v", got)
	}
	if first.Stats.DespawnSelected != 2 || first.Stats.SpawnSelected != 0 || first.Stats.SpawnDeferred != 1 {
		t.Fatalf("first churn stats=%+v", first.Stats)
	}
	for _, id := range []world.EntityID{3, 4} {
		svc.ConfirmDespawn(sid, id)
	}

	second := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := lifecycleIDs(second, false); len(got) != 0 {
		t.Fatalf("despawns repeated after confirmation: %v", got)
	}
	if got := lifecycleIDs(second, true); !equalIDs(got, []world.EntityID{5, 6}) {
		t.Fatalf("second churn spawns=%v want=[5 6]", got)
	}
	if second.Stats.SpawnSelected != 2 {
		t.Fatalf("second churn stats=%+v", second.Stats)
	}
}

func lifecycleFrame(tick uint64, count int) (*simulation.ReplicationFrame, []int) {
	ids := make([]world.EntityID, count)
	for i := range ids {
		ids[i] = world.EntityID(i + 1)
	}
	return lifecycleFrameIDs(tick, ids)
}

func lifecycleFrameIDs(tick uint64, ids []world.EntityID) (*simulation.ReplicationFrame, []int) {
	entities := make([]world.EntityState, len(ids))
	generations := make([]uint64, len(ids))
	indexByID := make(map[world.EntityID]int, len(ids))
	visible := make([]int, len(ids))
	for i, id := range ids {
		entities[i] = world.EntityState{ID: id, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: float32(i)}}}
		generations[i] = 1
		indexByID[id] = i
		visible[i] = i
	}
	return &simulation.ReplicationFrame{Tick: tick, Entities: entities, TransformGenerations: generations, IndexByID: indexByID}, visible
}
