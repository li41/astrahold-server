package replication

import (
	"sort"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildProducesSpawnRemoteSnapshotAndSelfCorrection(t *testing.T) {
	svc := NewService()
	sid := session.ID(7)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1}}},
		{ID: 2, Kind: world.EntityMonster, Transform: world.Transform{Position: world.Position{X: 2}}},
	}
	batch := svc.Build(sid, 1, 12, 20, visible)
	var spawn, snapshot, correction int
	for _, m := range batch.Messages {
		switch message := m.Message.(type) {
		case protocol.EntitySpawn:
			spawn++
		case protocol.WorldSnapshot:
			snapshot++
			if message.ChunkIndex != 0 || message.ChunkCount != 1 || len(message.Entities) != 1 || message.Entities[0].EntityID != 2 {
				t.Fatalf("unexpected snapshot chunk: %+v", message)
			}
		case protocol.PositionCorrection:
			correction++
			if message.EntityID != 1 || message.LastProcessedInputSequence != 12 {
				t.Fatalf("unexpected correction: %+v", message)
			}
		}
	}
	if spawn != 2 || snapshot != 1 || correction != 1 {
		t.Fatalf("unexpected counts spawn=%d snapshot=%d correction=%d", spawn, snapshot, correction)
	}
}

func TestBuildCapsSnapshotTransformsPerSession(t *testing.T) {
	svc := NewService()
	sid := session.ID(9)
	svc.Register(sid)
	visible := make([]world.EntityState, 100)
	for i := range visible {
		visible[i] = world.EntityState{
			ID: world.EntityID(i + 1),
			Kind: world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{X: float32(i)}},
		}
	}

	batch := svc.Build(sid, 1, 99, 50, visible)
	var chunks []protocol.WorldSnapshot
	for _, outbound := range batch.Messages {
		if snapshot, ok := outbound.Message.(protocol.WorldSnapshot); ok {
			chunks = append(chunks, snapshot)
		}
	}
	if batch.Stats.SnapshotCandidates != 99 || batch.Stats.SnapshotSelected != 64 || batch.Stats.SnapshotDeferred != 35 {
		t.Fatalf("unexpected stats: %+v", batch.Stats)
	}
	if len(chunks) != 2 {
		t.Fatalf("snapshot chunks=%d want=2", len(chunks))
	}
	wantSizes := []int{43, 21}
	for i, chunk := range chunks {
		if chunk.ChunkIndex != uint16(i) || chunk.ChunkCount != 2 || len(chunk.Entities) != wantSizes[i] {
			t.Fatalf("chunk[%d]=index:%d count:%d entities:%d", i, chunk.ChunkIndex, chunk.ChunkCount, len(chunk.Entities))
		}
		if !chunk.ValidChunk() {
			t.Fatalf("chunk[%d] should be valid", i)
		}
		for _, entity := range chunk.Entities {
			if entity.EntityID == 1 {
				t.Fatal("self transform must use PositionCorrection, not WorldSnapshot")
			}
		}
	}
}

func TestBuildAppliesTierCadenceToDirtyTransforms(t *testing.T) {
	policy := Policy{
		NearRadius: 10,
		MidRadius:  30,
		NearEveryBuilds: 1,
		MidEveryBuilds:  2,
		FarEveryBuilds:  5,
		NearRefreshEveryBuilds: 100,
		MidRefreshEveryBuilds:  100,
		FarRefreshEveryBuilds:  100,
		MaxTransformsPerBuild: 10,
	}
	svc := NewService(policy)
	sid := session.ID(12)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 5}}},
		{ID: 3, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 20}}},
		{ID: 4, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 50}}},
	}

	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 1, visible)); !equalIDs(got, []world.EntityID{2, 3, 4}) {
		t.Fatalf("build1 ids=%v", got)
	}
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 2, visible)); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("build2 ids=%v want near only", got)
	}
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 3, visible)); !equalIDs(got, []world.EntityID{2, 3}) {
		t.Fatalf("build3 ids=%v want near+mid", got)
	}
	moveRemotes(visible)
	_ = svc.Build(sid, 1, 0, 4, visible)
	moveRemotes(visible)
	_ = svc.Build(sid, 1, 0, 5, visible)
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 6, visible)); !containsID(got, 4) {
		t.Fatalf("build6 ids=%v want far entity due at cadence 5", got)
	}
}

func TestBuildBudgetUsesOverdueFairness(t *testing.T) {
	policy := Policy{
		NearRadius: 100,
		MidRadius:  200,
		NearEveryBuilds: 1,
		MidEveryBuilds:  1,
		FarEveryBuilds:  1,
		NearRefreshEveryBuilds: 100,
		MidRefreshEveryBuilds:  100,
		FarRefreshEveryBuilds:  100,
		MaxTransformsPerBuild: 2,
	}
	svc := NewService(policy)
	sid := session.ID(13)
	svc.Register(sid)
	visible := make([]world.EntityState, 7)
	for i := range visible {
		visible[i] = world.EntityState{ID: world.EntityID(i + 1), Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: float32(i)}}}
	}

	seen := make(map[world.EntityID]bool)
	for build := uint64(1); build <= 3; build++ {
		for _, id := range snapshotEntityIDs(svc.Build(sid, 1, 0, build, visible)) {
			seen[id] = true
		}
		moveRemotes(visible)
	}
	for id := world.EntityID(2); id <= 7; id++ {
		if !seen[id] {
			t.Fatalf("entity %d starved by budget; seen=%v", id, seen)
		}
	}
}

func TestBuildSkipsCleanTransformUntilPeriodicRefresh(t *testing.T) {
	policy := Policy{
		NearRadius: 10,
		MidRadius:  20,
		NearEveryBuilds: 1,
		MidEveryBuilds:  2,
		FarEveryBuilds:  5,
		NearRefreshEveryBuilds: 3,
		MidRefreshEveryBuilds:  6,
		FarRefreshEveryBuilds:  10,
		MaxTransformsPerBuild: 10,
	}
	svc := NewService(policy)
	sid := session.ID(14)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 5}}},
	}

	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 1, visible)); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("build1 ids=%v", got)
	}
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 2, visible)); len(got) != 0 {
		t.Fatalf("build2 clean ids=%v want none", got)
	}
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 3, visible)); len(got) != 0 {
		t.Fatalf("build3 clean ids=%v want none", got)
	}
	batch := svc.Build(sid, 1, 0, 4, visible)
	if got := snapshotEntityIDs(batch); !equalIDs(got, []world.EntityID{2}) || batch.Stats.ForcedRefreshCandidates != 1 {
		t.Fatalf("build4 refresh ids=%v stats=%+v", got, batch.Stats)
	}
}

func TestBuildKeepsReliableOrderForUnsortedCaller(t *testing.T) {
	svc := NewService()
	sid := session.ID(11)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 3, Kind: world.EntityPlayer},
		{ID: 1, Kind: world.EntityPlayer},
		{ID: 2, Kind: world.EntityPlayer},
	}
	batch := svc.Build(sid, 1, 0, 1, visible)
	var spawned []world.EntityID
	for _, outbound := range batch.Messages {
		if spawn, ok := outbound.Message.(protocol.EntitySpawn); ok {
			spawned = append(spawned, spawn.EntityID)
		}
	}
	if !equalIDs(spawned, []world.EntityID{1, 2, 3}) {
		t.Fatalf("spawn order=%v", spawned)
	}
}

func TestPolicyValidation(t *testing.T) {
	policy := DefaultPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("default policy invalid: %v", err)
	}
	policy.MidRadius = policy.NearRadius
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid radius policy")
	}
}

func snapshotEntityIDs(batch Batch) []world.EntityID {
	var ids []world.EntityID
	for _, outbound := range batch.Messages {
		if snapshot, ok := outbound.Message.(protocol.WorldSnapshot); ok {
			for _, entity := range snapshot.Entities {
				ids = append(ids, entity.EntityID)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func moveRemotes(visible []world.EntityState) {
	for i := range visible {
		if visible[i].ID == 1 {
			continue
		}
		visible[i].Transform.Position.Z += 0.25
	}
}

func equalIDs(a, b []world.EntityID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsID(ids []world.EntityID, target world.EntityID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
