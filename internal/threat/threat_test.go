package threat

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestTableAccumulatesAndSelectsHighestThreat(t *testing.T) {
	table := New()
	table.Add(20, 10)
	table.Add(10, 5)
	table.Add(10, 15)

	if got := table.Value(10); got != 20 {
		t.Fatalf("entity 10 threat = %d, want 20", got)
	}
	id, value, ok := table.HighestValid(nil)
	if !ok || id != 10 || value != 20 {
		t.Fatalf("highest = (%d,%d,%v), want (10,20,true)", id, value, ok)
	}
}

func TestTableUsesStableEntityIDTieBreak(t *testing.T) {
	table := New()
	table.Add(90, 40)
	table.Add(12, 40)
	table.Add(50, 40)

	id, value, ok := table.HighestValid(nil)
	if !ok || id != 12 || value != 40 {
		t.Fatalf("highest = (%d,%d,%v), want lower EntityID tie winner", id, value, ok)
	}
}

func TestHighestValidPrunesInvalidTargets(t *testing.T) {
	table := New()
	table.Add(1, 100)
	table.Add(2, 50)

	id, value, ok := table.HighestValid(func(id world.EntityID) bool { return id == 2 })
	if !ok || id != 2 || value != 50 {
		t.Fatalf("highest valid = (%d,%d,%v)", id, value, ok)
	}
	if table.Value(1) != 0 || table.Len() != 1 {
		t.Fatalf("invalid target was not pruned: len=%d value1=%d", table.Len(), table.Value(1))
	}
}

func TestTableSaturatesAndClears(t *testing.T) {
	table := New()
	table.Add(7, ^uint64(0)-2)
	table.Add(7, 10)
	if got := table.Value(7); got != ^uint64(0) {
		t.Fatalf("saturated threat = %d", got)
	}
	table.Remove(7)
	if table.Len() != 0 {
		t.Fatalf("len after remove = %d", table.Len())
	}
	table.Add(8, 1)
	table.Add(9, 2)
	table.Clear()
	if table.Len() != 0 {
		t.Fatalf("len after clear = %d", table.Len())
	}
}
