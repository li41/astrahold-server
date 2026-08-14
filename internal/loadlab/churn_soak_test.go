package loadlab

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTeleportChurnRestoreTargetsReturnMoversToOriginalSlots(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	const clients = 100
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
	if len(swap) != clients/2 || len(restore) != clients/2 {
		t.Fatalf("swap=%d restore=%d want=%d", len(swap), len(restore), clients/2)
	}

	groupSize := clients / 2
	moversPerGroup := groupSize / 2
	for local := 0; local < moversPerGroup; local++ {
		westID := world.EntityID(local + 1)
		eastID := world.EntityID(groupSize + local + 1)
		westInitial := factory(session.ID(westID), westID).Entity.Transform.Position
		eastInitial := factory(session.ID(eastID), eastID).Entity.Transform.Position

		if swap[westID] != eastInitial || swap[eastID] != westInitial {
			t.Fatalf("swap plan does not exchange matching slots west=%d east=%d", westID, eastID)
		}
		if restore[westID] != westInitial {
			t.Fatalf("west restore target=%+v want=%+v", restore[westID], westInitial)
		}
		if restore[eastID] != eastInitial {
			t.Fatalf("east restore target=%+v want=%+v", restore[eastID], eastInitial)
		}
	}
}
