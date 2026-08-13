package loadlab

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSnapshotAssemblyCompletesOutOfOrderChunks(t *testing.T) {
	var assembly snapshotAssembly
	chunk1 := protocol.WorldSnapshot{
		Tick: 10, ChunkIndex: 1, ChunkCount: 2,
		Entities: []protocol.EntityTransform{{EntityID: 2, Tick: 10, Position: world.Position{X: 2}}},
	}
	chunk0 := protocol.WorldSnapshot{
		Tick: 10, ChunkIndex: 0, ChunkCount: 2,
		Entities: []protocol.EntityTransform{{EntityID: 1, Tick: 10, Position: world.Position{X: 1}}},
	}
	if complete, reset := assembly.Accept(chunk1); complete || reset {
		t.Fatalf("first chunk complete=%v reset=%v", complete, reset)
	}
	if complete, reset := assembly.Accept(chunk0); !complete || reset {
		t.Fatalf("second chunk complete=%v reset=%v", complete, reset)
	}
	if assembly.lastCompleteTick != 10 {
		t.Fatalf("lastCompleteTick=%d", assembly.lastCompleteTick)
	}
}

func TestSnapshotAssemblyResetsIncompleteOlderTick(t *testing.T) {
	var assembly snapshotAssembly
	old := protocol.WorldSnapshot{Tick: 10, ChunkIndex: 0, ChunkCount: 2}
	newer := protocol.WorldSnapshot{Tick: 12, ChunkIndex: 0, ChunkCount: 1}
	if complete, reset := assembly.Accept(old); complete || reset {
		t.Fatalf("old chunk complete=%v reset=%v", complete, reset)
	}
	if complete, reset := assembly.Accept(newer); !complete || !reset {
		t.Fatalf("new chunk complete=%v reset=%v", complete, reset)
	}
}

func TestSnapshotAssemblyIgnoresCompletedOrOlderTick(t *testing.T) {
	var assembly snapshotAssembly
	current := protocol.WorldSnapshot{Tick: 10, ChunkIndex: 0, ChunkCount: 1}
	if complete, _ := assembly.Accept(current); !complete {
		t.Fatal("current snapshot should complete")
	}
	if complete, reset := assembly.Accept(current); complete || reset {
		t.Fatalf("duplicate completed snapshot complete=%v reset=%v", complete, reset)
	}
	older := protocol.WorldSnapshot{Tick: 8, ChunkIndex: 0, ChunkCount: 1}
	if complete, reset := assembly.Accept(older); complete || reset {
		t.Fatalf("older snapshot complete=%v reset=%v", complete, reset)
	}
}
