package gameplayworld

import "testing"

func TestMap0GMRoomFixtureOwnsIsolatedCollision(t *testing.T) {
	loaded, err := LoadFile("../../worlds/gm-room/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.WorldID != "gm-room" {
		t.Fatalf("world_id=%q want=gm-room", loaded.Definition.WorldID)
	}
	map0, ok := loaded.Definition.Map(MapIDGMRoom)
	if !ok {
		t.Fatal("map0 missing")
	}
	if map0.Kind != MapKindIsolated {
		t.Fatalf("map0 kind=%q want=%q", map0.Kind, MapKindIsolated)
	}
	if len(loaded.Definition.Blockers) != 15 {
		t.Fatalf("blockers=%d want=15", len(loaded.Definition.Blockers))
	}
	for _, blocker := range loaded.Definition.Blockers {
		if !blocker.Enabled || !blocker.BlocksMovement || !blocker.BlocksLOS {
			t.Fatalf("blocker %q enabled=%v movement=%v los=%v", blocker.ID, blocker.Enabled, blocker.BlocksMovement, blocker.BlocksLOS)
		}
	}
}
