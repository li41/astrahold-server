package loadlab

import "testing"

func TestS3E9MixedFixtureHotPairsAreBalancedAcrossClusters(t *testing.T) {
	pairs, err := TeleportChurnCombatPairs(500, 48)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 96 {
		t.Fatalf("combat pairs=%d want=96", len(pairs))
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
	if hot != 48 || west != 24 || east != 24 {
		t.Fatalf("hot pairs=%d west=%d east=%d want=48/24/24", hot, west, east)
	}
}

func TestS3E9MixedFixtureInterleavesMoversAndStationaryObservers(t *testing.T) {
	stationary := 0
	moving := 0
	for entityID := uint64(1); entityID <= 500; entityID++ {
		if S3E9MixedStationaryEntity(worldEntityID(entityID)) {
			stationary++
		} else {
			moving++
		}
	}
	if stationary != 250 || moving != 250 {
		t.Fatalf("stationary=%d moving=%d want=250/250", stationary, moving)
	}
}

// worldEntityID keeps this fixture assertion explicit without coupling the production helper to raw uint64 callers.
func worldEntityID(value uint64) world.EntityID { return world.EntityID(value) }
