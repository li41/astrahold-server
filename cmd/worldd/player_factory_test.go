package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestFreshPlayerCanReachMainGateFrontInCastleSandbox(t *testing.T) {
	loadedWorld, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	loadedRespawn, err := respawnpolicy.LoadFile("../../config/respawn-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := respawnpolicy.ValidateAgainstWorld(loadedRespawn.Definition, loadedWorld.Definition); err != nil {
		t.Fatal(err)
	}

	spawn, err := freshPlayerSpawn(loadedRespawn.Definition)
	if err != nil {
		t.Fatal(err)
	}
	if spawn.ID != "field-camp" || spawn.Layer != 0 || spawn.Z != -35 {
		t.Fatalf("fresh spawn=%#v; want field-camp on layer 0 at z=-35", spawn)
	}

	factory := newWorldPlayerFactory(spawn, loadedWorld.Definition.Agent)
	spec := factory(1, 1)
	state := movement.AgentState{
		Position:      spec.Entity.Transform.Position,
		Speed:         spec.Speed,
		Radius:        spec.Radius,
		MaxStepHeight: spec.MaxStepHeight,
	}

	nav, err := navigation.NewGameplayNavigator(loadedWorld.Definition)
	if err != nil {
		t.Fatal(err)
	}
	move := movement.NewService(nav, 0.1)
	if err := move.AcceptInput(&state, movement.Input{Direction: world.Vec3{Z: 1}}); err != nil {
		t.Fatal(err)
	}

	const gateFrontZ = float32(8.5)
	for step := 0; step < 100 && state.Position.Z < gateFrontZ; step++ {
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
	if state.Position.Z < gateFrontZ {
		t.Fatalf("stopped at z=%g; want to reach main-gate front z>=%g", state.Position.Z, gateFrontZ)
	}
}
