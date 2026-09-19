package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/playerarchetype"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestFreshPlayerUsesWorldMasterSpawnAndCanReachEmberwatchCenter(t *testing.T) {
	loadedWorld, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	loadedRespawn, err := respawnpolicy.LoadFile("../../config/respawn-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	heightfields, err := loadWorldHeightfields("../../worlds/castle-sandbox/gameplay.json", loadedWorld.Definition)
	if err != nil {
		t.Fatal(err)
	}
	nav, err := navigation.NewGameplayNavigatorWithHeightfields(loadedWorld.Definition, heightfields)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRespawn, err := respawnpolicy.ResolveGroundPositions(loadedRespawn.Definition, loadedWorld.Definition, nav)
	if err != nil {
		t.Fatal(err)
	}

	spawn, err := freshPlayerSpawn(resolvedRespawn)
	if err != nil {
		t.Fatal(err)
	}
	if spawn.ID != "field-camp" || spawn.Layer != 0 || spawn.X != 0 || spawn.Z != -220 {
		t.Fatalf("fresh spawn=%#v; want field-camp on layer 0 at (0,-220)", spawn)
	}
	if spawn.Y == 0 {
		t.Fatalf("fresh spawn y=%g; want authoritative Map1 terrain height", spawn.Y)
	}

	factory := newWorldPlayerFactory(spawn, loadedWorld.Definition.Agent)
	spec := factory(1, 1)
	if spec.Entity.ArchetypeID != playerarchetype.DefaultID {
		t.Fatalf("player archetype=%q; want %q", spec.Entity.ArchetypeID, playerarchetype.DefaultID)
	}
	if spec.Speed != defaultPlayerGroundSpeedMetersPerSecond {
		t.Fatalf("player speed=%g; want authoritative normal ground speed=%g", spec.Speed, defaultPlayerGroundSpeedMetersPerSecond)
	}
	state := movement.AgentState{
		Position:      spec.Entity.Transform.Position,
		Speed:         spec.Speed,
		Radius:        spec.Radius,
		MaxStepHeight: spec.MaxStepHeight,
	}

	move := movement.NewService(nav, 0.1)
	if err := move.AcceptInput(&state, movement.Input{Direction: world.Vec3{Z: 1}}); err != nil {
		t.Fatal(err)
	}

	const villageCenterZ = float32(0)
	for step := 0; step < 600 && state.Position.Z < villageCenterZ; step++ {
		before := state.Position.Z
		if _, err := move.Step(&state, 0.1); err != nil {
			t.Fatalf("step %d from z=%g: %v", step, before, err)
		}
		if state.Position.Z <= before {
			t.Fatalf("step %d did not advance: before=%g after=%g", step, before, state.Position.Z)
		}
	}
	if state.Position.Layer != 0 {
		t.Fatalf("layer=%d; want ground layer 0", state.Position.Layer)
	}
	if state.Position.Z < villageCenterZ {
		t.Fatalf("stopped at z=%g; want to reach Emberwatch center z>=%g", state.Position.Z, villageCenterZ)
	}
}
