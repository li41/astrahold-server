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
	if first.Stats.SpawnCandidates != 5 || first.Stats.SpawnSelected != 2 || first.Stats.SpawnDeferred != 3 {
		t.Fatalf("first spawn stats=%+v", first.Stats)
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
		t.Fatalf("second spawn stats=%+v", second.Stats)
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
	fourth := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, limits)
	if got := snapshotEntityIDs(fourth); !equalIDs(got, []world.EntityID{2, 3, 4, 5}) {
		t.Fatalf("converged view snapshot ids=%v want=[2 3 4 5]", got)
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

func lifecycleFrame(tick uint64, count int) (*simulation.ReplicationFrame, []int) {
	entities := make([]world.EntityState, count)
	generations := make([]uint64, count)
	indexByID := make(map[world.EntityID]int, count)
	visible := make([]int, count)
	for i := range entities {
		id := world.EntityID(i + 1)
		entities[i] = world.EntityState{ID: id, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: float32(i)}}}
		generations[i] = 1
		indexByID[id] = i
		visible[i] = i
	}
	return &simulation.ReplicationFrame{Tick: tick, Entities: entities, TransformGenerations: generations, IndexByID: indexByID}, visible
}
