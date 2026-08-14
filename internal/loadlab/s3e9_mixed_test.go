package loadlab

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestS3E9MixedFixtureHotPairsAreBalancedAcrossClusters(t *testing.T) {
	pairs, err := TeleportChurnCombatPairs(500, 56)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 112 {
		t.Fatalf("combat pairs=%d want=112", len(pairs))
	}

	hot := 0
	west := 0
	east := 0
	for _, pair := range pairs {
		if !S3E9MixedStationaryEntity(pair.ActorID) || !S3E9MixedStationaryEntity(pair.TargetID) {
			continue
		}
		hot++
		if pair.ActorID <= 250 {
			west++
		} else {
			east++
		}
	}
	if hot != 56 || west != 28 || east != 28 {
		t.Fatalf("hot pairs=%d west=%d east=%d want=56/28/28", hot, west, east)
	}
}

func TestS3E9MixedFixtureInterleavesMoversAndStationaryObservers(t *testing.T) {
	stationary := 0
	moving := 0
	for entityID := world.EntityID(1); entityID <= 500; entityID++ {
		if S3E9MixedStationaryEntity(entityID) {
			stationary++
		} else {
			moving++
		}
	}
	if stationary != 250 || moving != 250 {
		t.Fatalf("stationary=%d moving=%d want=250/250", stationary, moving)
	}
}
