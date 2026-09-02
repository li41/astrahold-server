package replication

import (
	"sort"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildProducesReliableSpawnBeforeRemoteSnapshot(t *testing.T) {
	svc := NewService()
	sid := session.ID(7)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1}}},
		{ID: 2, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01", Transform: world.Transform{Position: world.Position{X: 2}}},
	}

	first := svc.Build(sid, 1, 12, 20, visible)
	var spawn int
	for _, outbound := range first.Messages {
		switch message := outbound.Message.(type) {
		case protocol.EntitySpawn:
			spawn++
			if message.EntityID == 2 && message.ArchetypeID != "wolf-gray-01" {
				t.Fatalf("monster archetype=%q want wolf-gray-01", message.ArchetypeID)
			}
			if svc.Knows(sid, message.EntityID) {
				t.Fatalf("entity %d became known before delivery confirmation", message.EntityID)
			}
		case protocol.WorldSnapshot:
			if len(message.Entities) != 0 {
				t.Fatalf("unknown entities must not enter realtime snapshot: %+v", message)
			}
		}
	}
	if spawn != 2 {
		t.Fatalf("spawn=%d want=2", spawn)
	}

	svc.ConfirmSpawn(sid, 1)
	svc.ConfirmSpawn(sid, 2)
	second := svc.Build(sid, 1, 12, 21, visible)
	var snapshot, correction int
	for _, outbound := range second.Messages {
		switch message := outbound.Message.(type) {
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
	if snapshot != 1 || correction != 1 {
		t.Fatalf("snapshot=%d correction=%d want=1/1", snapshot, correction)
	}
}

func TestLifecycleRequiresDeliveryConfirmationAndRetries(t *testing.T) {
	svc := NewService()
	sid := session.ID(8)
	svc.Register(sid)
	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer},
		{ID: 2, Kind: world.EntityPlayer},
	}

	first := svc.Build(sid, 1, 0, 1, visible)
	if got := lifecycleIDs(first, true); !equalIDs(got, []world.EntityID{1, 2}) {
		t.Fatalf("first spawn ids=%v", got)
	}
	second := svc.Build(sid, 1, 0, 2, visible)
	if got := lifecycleIDs(second, true); !equalIDs(got, []world.EntityID{1, 2}) {
		t.Fatalf("unconfirmed spawns were not retried: %v", got)
	}

	svc.ConfirmSpawn(sid, 1)
	svc.ConfirmSpawn(sid, 2)
	third := svc.Build(sid, 1, 0, 3, visible)
	if got := lifecycleIDs(third, true); len(got) != 0 {
		t.Fatalf("confirmed spawns repeated: %v", got)
	}
	if !svc.Knows(sid, 1) || !svc.Knows(sid, 2) {
		t.Fatal("confirmed entities should be known")
	}

	departed := svc.Build(sid, 1, 0, 4, visible[:1])
	if got := lifecycleIDs(departed, false); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("despawn ids=%v want=[2]", got)
	}
	if !svc.Knows(sid, 2) {
		t.Fatal("entity must remain known until despawn delivery succeeds")
	}
	retry := svc.Build(sid, 1, 0, 5, visible[:1])
	if got := lifecycleIDs(retry, false); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("unconfirmed despawn was not retried: %v", got)
	}
	svc.ConfirmDespawn(sid, 2)
	if svc.Knows(sid, 2) {
		t.Fatal("confirmed despawn should clear known")
	}
	settled := svc.Build(sid, 1, 0, 6, visible[:1])
	if got := lifecycleIDs(settled, false); len(got) != 0 {
		t.Fatalf("confirmed despawn repeated: %v", got)
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
	primeKnown(svc, sid, 1, visible)

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
	primeKnown(svc, sid, 1, visible)

	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 2, visible)); !equalIDs(got, []world.EntityID{2, 3, 4}) {
		t.Fatalf("initial known batch ids=%v", got)
	}
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 3, visible)); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("next batch ids=%v want near only", got)
	}
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 4, visible)); !equalIDs(got, []world.EntityID{2, 3}) {
		t.Fatalf("mid cadence ids=%v want near+mid", got)
	}
	moveRemotes(visible)
	_ = svc.Build(sid, 1, 0, 5, visible)
	moveRemotes(visible)
	_ = svc.Build(sid, 1, 0, 6, visible)
	moveRemotes(visible)
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 7, visible)); !containsID(got, 4) {
		t.Fatalf("far entity not due after five builds: %v", got)
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
	primeKnown(svc, sid, 1, visible)

	seen := make(map[world.EntityID]bool)
	for build := uint64(2); build <= 4; build++ {
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
	primeKnown(svc, sid, 1, visible)

	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 2, visible)); !equalIDs(got, []world.EntityID{2}) {
		t.Fatalf("initial ids=%v", got)
	}
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 3, visible)); len(got) != 0 {
		t.Fatalf("clean ids=%v want none", got)
	}
	if got := snapshotEntityIDs(svc.Build(sid, 1, 0, 4, visible)); len(got) != 0 {
		t.Fatalf("clean ids=%v want none", got)
	}
	batch := svc.Build(sid, 1, 0, 5, visible)
	if got := snapshotEntityIDs(batch); !equalIDs(got, []world.EntityID{2}) || batch.Stats.ForcedRefreshCandidates != 1 {
		t.Fatalf("refresh ids=%v stats=%+v", got, batch.Stats)
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

func primeKnown(svc *Service, sid session.ID, selfID world.EntityID, visible []world.EntityState) {
	_ = svc.Build(sid, selfID, 0, 1, visible)
	for _, entity := range visible {
		svc.ConfirmSpawn(sid, entity.ID)
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

func lifecycleIDs(batch Batch, spawn bool) []world.EntityID {
	var ids []world.EntityID
	for _, outbound := range batch.Messages {
		switch message := outbound.Message.(type) {
		case protocol.EntitySpawn:
			if spawn {
				ids = append(ids, message.EntityID)
			}
		case protocol.EntityDespawn:
			if !spawn {
				ids = append(ids, message.EntityID)
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
