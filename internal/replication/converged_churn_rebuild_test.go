package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestRebuildConvergedLifecycleTracksMergesDenseStateWithoutMapRebuild(t *testing.T) {
	state := newViewState()
	state.desiredIDs = []world.EntityID{1, 2, 3, 4}
	state.tracks = []entityTrack{
		{id: 1, known: true, lastDeliveredGeneration: 11, lastSentBuild: 21},
		{id: 2, known: true, lastDeliveredGeneration: 12, lastSentBuild: 22},
		{id: 3, known: true, lastDeliveredGeneration: 13, lastSentBuild: 23},
		{id: 4, known: true, lastDeliveredGeneration: 14, lastSentBuild: 24},
	}
	for _, id := range state.desiredIDs {
		state.known[id] = struct{}{}
	}

	frame, visible := lifecycleFrameIDs(50, []world.EntityID{1, 2, 5, 6})
	rebuildConvergedLifecycleTracks(state, frame, visible)

	if got := state.departed; !equalIDs(got, []world.EntityID{3, 4}) {
		t.Fatalf("departed=%v want=[3 4]", got)
	}
	if got := state.desiredIDs; !equalIDs(got, []world.EntityID{1, 2, 5, 6}) {
		t.Fatalf("desired=%v want=[1 2 5 6]", got)
	}
	if !state.tracks[0].known || state.tracks[0].lastDeliveredGeneration != 11 || state.tracks[0].lastSentBuild != 21 {
		t.Fatalf("retained track 1=%+v", state.tracks[0])
	}
	if !state.tracks[1].known || state.tracks[1].lastDeliveredGeneration != 12 || state.tracks[1].lastSentBuild != 22 {
		t.Fatalf("retained track 2=%+v", state.tracks[1])
	}
	for i := 2; i < 4; i++ {
		if state.tracks[i].known || state.tracks[i].lastDeliveredGeneration != 0 || state.tracks[i].lastSentBuild != 0 {
			t.Fatalf("new track %d=%+v want unknown zero history", i, state.tracks[i])
		}
	}
}
