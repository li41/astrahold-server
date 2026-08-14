package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildFrameBorrowedReusesSnapshotBackingStorage(t *testing.T) {
	svc := NewService()
	sid := session.ID(41)
	svc.Register(sid)
	frame := &simulation.ReplicationFrame{
		Tick: 1,
		Entities: []world.EntityState{
			{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0}}},
			{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2}}},
		},
		TransformGenerations: []uint64{1, 1},
		IndexByID: map[world.EntityID]int{1: 0, 2: 1},
	}
	visible := []int{0, 1}

	_ = svc.BuildFrameBorrowed(sid, 1, 0, frame, visible)
	svc.ConfirmSpawn(sid, 1)
	svc.ConfirmSpawn(sid, 2)

	frame.Tick = 2
	second := svc.BuildFrameBorrowed(sid, 1, 0, frame, visible)
	secondSnapshot := firstSnapshot(second)
	if secondSnapshot == nil || len(secondSnapshot.Entities) != 1 {
		t.Fatalf("second snapshot=%+v", secondSnapshot)
	}
	backing := &secondSnapshot.Entities[0]

	frame.Tick = 3
	frame.Entities[1].Transform.Position.X = 3
	frame.TransformGenerations[1] = 2
	third := svc.BuildFrameBorrowed(sid, 1, 0, frame, visible)
	thirdSnapshot := firstSnapshot(third)
	if thirdSnapshot == nil || len(thirdSnapshot.Entities) != 1 {
		t.Fatalf("third snapshot=%+v", thirdSnapshot)
	}
	if &thirdSnapshot.Entities[0] != backing {
		t.Fatal("borrowed snapshot did not reuse per-session backing storage")
	}
	if thirdSnapshot.Entities[0].Position.X != 3 {
		t.Fatalf("third snapshot position=%v want=3", thirdSnapshot.Entities[0].Position.X)
	}
}

func TestRebuildDesiredTracksReusesDenseBuffersWhenCapacityFits(t *testing.T) {
	state := newViewState()
	frame := &simulation.ReplicationFrame{
		Entities: []world.EntityState{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	visible := []int{0, 1, 2}
	rebuildDesiredTracks(state, frame, visible)
	if len(state.desiredIDs) != 3 || len(state.tracks) != 3 {
		t.Fatalf("unexpected initial dense lengths: ids=%d tracks=%d", len(state.desiredIDs), len(state.tracks))
	}
	idsBacking := &state.desiredIDs[0]
	tracksBacking := &state.tracks[0]

	state.known[2] = struct{}{}
	state.lastDeliveredGeneration[2] = 7
	state.lastSentBuild[2] = 9
	rebuildDesiredTracks(state, frame, visible)
	if &state.desiredIDs[0] != idsBacking || &state.tracks[0] != tracksBacking {
		t.Fatal("membership rebuild replaced dense backing buffers despite sufficient capacity")
	}
	if !state.tracks[1].known || state.tracks[1].lastDeliveredGeneration != 7 || state.tracks[1].lastSentBuild != 9 {
		t.Fatalf("rebuilt track lost state: %+v", state.tracks[1])
	}
}

func firstSnapshot(batch Batch) *protocol.WorldSnapshot {
	for _, outbound := range batch.Messages {
		if snapshot, ok := outbound.Message.(protocol.WorldSnapshot); ok {
			return &snapshot
		}
	}
	return nil
}
