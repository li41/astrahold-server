package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRebuildPendingDepartedFromDesiredDiffUsesSortedKnownRemovedIDs(t *testing.T) {
	state := newViewState()
	state.desiredIDs = []world.EntityID{1, 2, 3, 4, 6}
	state.known[1] = struct{}{}
	state.known[2] = struct{}{}
	// 3 刻意 unknown：即使離開 desired，也不能送 Despawn。
	state.known[4] = struct{}{}
	state.known[6] = struct{}{}

	frame := &simulation.ReplicationFrame{
		Entities: []world.EntityState{
			{ID: 1},
			{ID: 3},
			{ID: 5},
			{ID: 6},
		},
	}
	visible := []int{0, 1, 2, 3}
	rebuildPendingDepartedFromDesiredDiff(state, frame, visible)

	if !equalIDs(state.departed, []world.EntityID{2, 4}) {
		t.Fatalf("departed=%v want=[2 4]", state.departed)
	}
}
