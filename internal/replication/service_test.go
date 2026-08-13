package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildProducesSpawnSnapshotAndCorrection(t *testing.T) {
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
			if message.ChunkIndex != 0 || message.ChunkCount != 1 || len(message.Entities) != 2 {
				t.Fatalf("unexpected snapshot chunk: %+v", message)
			}
		case protocol.PositionCorrection:
			correction++
		}
	}
	if spawn != 2 || snapshot != 1 || correction != 1 {
		t.Fatalf("unexpected counts spawn=%d snapshot=%d correction=%d", spawn, snapshot, correction)
	}
}

func TestBuildChunksLargeSnapshotWithinProtocolLimit(t *testing.T) {
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
	if len(chunks) != 3 {
		t.Fatalf("snapshot chunks=%d want=3", len(chunks))
	}
	wantSizes := []int{43, 43, 14}
	for i, chunk := range chunks {
		if chunk.ChunkIndex != uint16(i) || chunk.ChunkCount != 3 || len(chunk.Entities) != wantSizes[i] {
			t.Fatalf("chunk[%d]=index:%d count:%d entities:%d", i, chunk.ChunkIndex, chunk.ChunkCount, len(chunk.Entities))
		}
		if !chunk.ValidChunk() {
			t.Fatalf("chunk[%d] should be valid", i)
		}
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
	if len(spawned) != 3 || spawned[0] != 1 || spawned[1] != 2 || spawned[2] != 3 {
		t.Fatalf("spawn order=%v", spawned)
	}
}
