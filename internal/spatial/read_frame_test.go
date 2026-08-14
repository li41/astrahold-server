package spatial

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestReadFrameReusesCellCandidatesButKeepsSessionFiltering(t *testing.T) {
	entities := []world.EntityState{
		{ID: 1, Transform: world.Transform{Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0}}},
		{ID: 2, Transform: world.Transform{Position: world.Position{X: 3, Y: 0, Z: 0, Layer: 0}}},
		{ID: 3, Transform: world.Transform{Position: world.Position{X: 3, Y: 8, Z: 0, Layer: 2}}},
		{ID: 4, Transform: world.Transform{Position: world.Position{X: 40, Y: 0, Z: 0, Layer: 0}}},
	}
	var frame ReadFrame
	frame.Reset(32, entities)

	first, firstStats := frame.QueryRadiusInto(world.Position{Layer: 0}, 10, QueryOptions{SameLayer: true}, nil)
	if firstStats.SharedCandidateBuilds != 1 || firstStats.SharedCandidateReuses != 0 || firstStats.SharedCandidateScans == 0 {
		t.Fatalf("unexpected first shared stats: %+v", firstStats)
	}
	if len(first) != 2 || entities[first[0]].ID != 1 || entities[first[1]].ID != 2 {
		t.Fatalf("first visible indexes=%v", first)
	}

	second, secondStats := frame.QueryRadiusInto(world.Position{X: 1, Layer: 2}, 10, QueryOptions{SameLayer: true}, first[:0])
	if secondStats.SharedCandidateBuilds != 0 || secondStats.SharedCandidateReuses != 1 || secondStats.SharedCandidateScans != 0 {
		t.Fatalf("unexpected second shared stats: %+v", secondStats)
	}
	if len(second) != 1 || entities[second[0]].ID != 3 {
		t.Fatalf("second visible indexes=%v", second)
	}
}
