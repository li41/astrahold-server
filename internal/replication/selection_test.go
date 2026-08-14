package replication

import (
	"sort"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestSelectSnapshotCandidatesMatchesFullPrioritySort(t *testing.T) {
	candidates := make([]snapshotCandidate, 180)
	for i := range candidates {
		id := world.EntityID(i + 1)
		candidates[i] = snapshotCandidate{
			entity:  world.EntityState{ID: id},
			tier:    Tier(i % 3),
			age:     uint64((i*17)%29 + 1),
			cadence: uint64((i%5)+1),
			dirty:   i%4 != 0,
		}
	}

	want := append([]snapshotCandidate(nil), candidates...)
	sort.Slice(want, func(i, j int) bool { return candidateHigherPriority(want[i], want[j]) })
	want = want[:64]

	got := selectSnapshotCandidates(nil, candidates, 64)
	if len(got) != len(want) {
		t.Fatalf("selected=%d want=%d", len(got), len(want))
	}

	wantIDs := candidateIDs(want)
	gotIDs := candidateIDs(got)
	sort.Slice(wantIDs, func(i, j int) bool { return wantIDs[i] < wantIDs[j] })
	sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("selected ids differ at %d: got=%v want=%v", i, gotIDs, wantIDs)
		}
	}
}

func TestSelectSnapshotCandidatesReusesTopKBuffer(t *testing.T) {
	candidates := make([]snapshotCandidate, 100)
	for i := range candidates {
		candidates[i] = snapshotCandidate{
			entity:  world.EntityState{ID: world.EntityID(i + 1)},
			age:     uint64(i + 1),
			cadence: 1,
			dirty:   true,
		}
	}
	buffer := make([]snapshotCandidate, 64)
	backing := &buffer[0]
	got := selectSnapshotCandidates(buffer, candidates, 64)
	if &got[0] != backing {
		t.Fatal("top-K selection replaced backing storage despite sufficient capacity")
	}
}

func candidateIDs(values []snapshotCandidate) []world.EntityID {
	ids := make([]world.EntityID, len(values))
	for i, value := range values {
		ids[i] = value.entity.ID
	}
	return ids
}
