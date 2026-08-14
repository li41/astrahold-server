package replication

import (
	"sort"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestSnapshotSelectionMatchesLegacyTopK(t *testing.T) {
	for _, count := range []int{0, 32, 64, 65, 500} {
		t.Run(testCountName(count), func(t *testing.T) {
			candidates := make([]snapshotCandidate, count)
			for i := range candidates {
				candidates[i] = snapshotCandidate{
					entity:     world.EntityState{ID: world.EntityID(i + 1)},
					generation: uint64(i + 10),
					trackIndex: i,
					tier:       TierNear + Tier(i%3),
					age:        uint64((i*17)%37 + 1),
					cadence:    []uint64{1, 2, 5}[i%3],
					dirty:      i%4 != 0,
				}
			}

			legacy := selectSnapshotCandidates(nil, candidates, 64)
			selection := newSnapshotSelection(nil, 64)
			for _, candidate := range candidates {
				selection.Consider(candidate)
			}
			online := selection.Selected()
			if selection.Count() != len(candidates) {
				t.Fatalf("count=%d want=%d", selection.Count(), len(candidates))
			}
			if len(online) != len(legacy) {
				t.Fatalf("selected=%d legacy=%d", len(online), len(legacy))
			}

			if count <= 64 {
				for i := range online {
					if online[i].entity.ID != legacy[i].entity.ID {
						t.Fatalf("order[%d]=%d legacy=%d", i, online[i].entity.ID, legacy[i].entity.ID)
					}
				}
				return
			}

			onlineIDs := candidateIDsSorted(online)
			legacyIDs := candidateIDsSorted(legacy)
			for i := range onlineIDs {
				if onlineIDs[i] != legacyIDs[i] {
					t.Fatalf("top-K set differs at %d: online=%d legacy=%d", i, onlineIDs[i], legacyIDs[i])
				}
			}
		})
	}
}

func candidateIDsSorted(candidates []snapshotCandidate) []world.EntityID {
	ids := make([]world.EntityID, len(candidates))
	for i, candidate := range candidates {
		ids[i] = candidate.entity.ID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func testCountName(count int) string {
	if count == 0 {
		return "zero"
	}
	const digits = "0123456789"
	var buffer [20]byte
	index := len(buffer)
	for value := count; value > 0; value /= 10 {
		index--
		buffer[index] = digits[value%10]
	}
	return string(buffer[index:])
}
