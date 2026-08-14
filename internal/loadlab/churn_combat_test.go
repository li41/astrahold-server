package loadlab

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/session"
)

func TestTeleportChurnCombatPairsRemainInBasicAttackRange(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	const clients = 500
	const pairsPerGroup = 32
	pairs, err := TeleportChurnCombatPairs(clients, pairsPerGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != pairsPerGroup*2 {
		t.Fatalf("pairs=%d want=%d", len(pairs), pairsPerGroup*2)
	}
	factory, err := NewPlayerFactory(loaded.Definition, ScenarioTeleportChurn, clients)
	if err != nil {
		t.Fatal(err)
	}
	swap, err := TeleportChurnTargets(loaded.Definition, clients)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := TeleportChurnRestoreTargets(loaded.Definition, clients)
	if err != nil {
		t.Fatal(err)
	}

	const rangeLimit = float32(4.5 * 4.5)
	seenActors := make(map[uint64]struct{}, len(pairs))
	seenTargets := make(map[uint64]struct{}, len(pairs))
	for _, pair := range pairs {
		if pair.ActorID == pair.TargetID || pair.ActorID == 0 || pair.TargetID == 0 {
			t.Fatalf("invalid pair=%+v", pair)
		}
		if _, ok := seenActors[uint64(pair.ActorID)]; ok {
			t.Fatalf("actor reused: %d", pair.ActorID)
		}
		if _, ok := seenTargets[uint64(pair.TargetID)]; ok {
			t.Fatalf("target reused: %d", pair.TargetID)
		}
		seenActors[uint64(pair.ActorID)] = struct{}{}
		seenTargets[uint64(pair.TargetID)] = struct{}{}

		actorInitial := factory(session.ID(pair.ActorID), pair.ActorID).Entity.Transform.Position
		targetInitial := factory(session.ID(pair.TargetID), pair.TargetID).Entity.Transform.Position
		if actorInitial.DistanceSquared(targetInitial) > rangeLimit {
			t.Fatalf("restore pair out of range actor=%d target=%d distance_sq=%f", pair.ActorID, pair.TargetID, actorInitial.DistanceSquared(targetInitial))
		}
		actorSwap, actorOK := swap[pair.ActorID]
		targetSwap, targetOK := swap[pair.TargetID]
		if !actorOK || !targetOK {
			t.Fatalf("combat pair must both be movers: actor=%d target=%d", pair.ActorID, pair.TargetID)
		}
		if actorSwap.DistanceSquared(targetSwap) > rangeLimit {
			t.Fatalf("swap pair out of range actor=%d target=%d distance_sq=%f", pair.ActorID, pair.TargetID, actorSwap.DistanceSquared(targetSwap))
		}
		if restore[pair.ActorID] != actorInitial || restore[pair.TargetID] != targetInitial {
			t.Fatalf("restore target mismatch actor=%d target=%d", pair.ActorID, pair.TargetID)
		}
	}
}
