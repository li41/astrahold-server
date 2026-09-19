package navigation

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCastleSandboxRetiredCastleRoadIsOpen(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	nav, err := NewGameplayNavigator(loaded.Definition)
	if err != nil {
		t.Fatal(err)
	}
	agent := Agent{Radius: loaded.Definition.Agent.Radius, MaxStepHeight: loaded.Definition.Agent.MaxStepHeight}

	start := world.Position{X: 0, Y: 0, Z: 8, Layer: 0}
	got, err := nav.ResolveMove(start, world.Vec3{Z: 4}, agent)
	if err != nil {
		t.Fatalf("retired castle road move error = %v", err)
	}
	if got.Layer != 0 || got.Z != 12 {
		t.Fatalf("retired castle road position = %+v, want layer=0 z=12", got)
	}
	if !nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 6, Layer: 0}, world.Position{X: 0, Y: 1, Z: 15, Layer: 0}) {
		t.Fatal("retired castle road LOS = false, want true")
	}
	if err := nav.SetBlockerEnabled("main-gate", false); err == nil {
		t.Fatal("retired main-gate unexpectedly exists")
	}
}

func TestCastleSandboxEmberwatchVillageBlockers(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Revision != "first-continent-map1-terrain-v1" {
		t.Fatalf("revision=%q, want first-continent-map1-terrain-v1", loaded.Definition.Revision)
	}

	type expectedBlocker struct {
		id        string
		bounds    gameplayworld.BoundsXZ
		minY      float32
		maxY      float32
		blocksLOS bool
	}
	expected := []expectedBlocker{
		{id: "map1-emberwatch-hall", bounds: gameplayworld.BoundsXZ{MinX: -30.5, MaxX: -13.5, MinZ: -19.61, MaxZ: -8.39}, minY: 44.02, maxY: 48.02, blocksLOS: true},
		{id: "map1-emberwatch-inn", bounds: gameplayworld.BoundsXZ{MinX: 20.23, MaxX: 27.77, MinZ: -0.31, MaxZ: 8.31}, minY: 45.36, maxY: 49.36, blocksLOS: true},
		{id: "map1-emberwatch-supply", bounds: gameplayworld.BoundsXZ{MinX: 36.68, MaxX: 47.32, MinZ: -27.79, MaxZ: -12.21}, minY: 45.32, maxY: 49.32, blocksLOS: true},
		{id: "map1-emberwatch-herbalist", bounds: gameplayworld.BoundsXZ{MinX: -50.13, MaxX: -40.87, MinZ: -1.46, MaxZ: 13.46}, minY: 46.2, maxY: 50.2, blocksLOS: true},
		{id: "map1-emberwatch-bakery", bounds: gameplayworld.BoundsXZ{MinX: -24.54, MaxX: -15.46, MinZ: 18.38, MaxZ: 25.62}, minY: 46.57, maxY: 50.57, blocksLOS: true},
		{id: "map1-emberwatch-notice", bounds: gameplayworld.BoundsXZ{MinX: 11.35, MaxX: 14.65, MinZ: -25.95, MaxZ: -24.05}, minY: 44.49, maxY: 47.49, blocksLOS: true},
		{id: "map1-emberwatch-watch-tree", bounds: gameplayworld.BoundsXZ{MinX: -43.1, MaxX: -32.9, MinZ: 32.9, MaxZ: 43.1}, minY: 47.58, maxY: 54.38, blocksLOS: true},
		{id: "map1-emberwatch-cottage-east", bounds: gameplayworld.BoundsXZ{MinX: 7.96, MaxX: 20.04, MinZ: 22.5, MaxZ: 29.5}, minY: 46.61, maxY: 49.61, blocksLOS: true},
		{id: "map1-emberwatch-cottage-north", bounds: gameplayworld.BoundsXZ{MinX: -16.04, MaxX: -3.96, MinZ: -55.25, MaxZ: -48.75}, minY: 41.59, maxY: 46.59, blocksLOS: true},
		{id: "map1-emberwatch-cottage-west", bounds: gameplayworld.BoundsXZ{MinX: -51.73, MaxX: -48.27, MinZ: -38.74, MaxZ: -29.26}, minY: 44.49, maxY: 47.49, blocksLOS: true},
		{id: "map1-emberwatch-cottage-tall", bounds: gameplayworld.BoundsXZ{MinX: 19.67, MaxX: 24.33, MinZ: -51.96, MaxZ: -48.04}, minY: 41.43, maxY: 44.43, blocksLOS: true},
		{id: "map1-emberwatch-cottage-south", bounds: gameplayworld.BoundsXZ{MinX: 48.94, MaxX: 55.06, MinZ: 7.85, MaxZ: 16.15}, minY: 46.32, maxY: 51.32, blocksLOS: true},
	}

	byID := make(map[string]gameplayworld.Blocker, len(loaded.Definition.Blockers))
	for _, blocker := range loaded.Definition.Blockers {
		byID[blocker.ID] = blocker
	}

	nav, err := NewGameplayNavigator(loaded.Definition)
	if err != nil {
		t.Fatal(err)
	}
	agent := Agent{Radius: loaded.Definition.Agent.Radius, MaxStepHeight: loaded.Definition.Agent.MaxStepHeight}

	for _, want := range expected {
		got, ok := byID[want.id]
		if !ok {
			t.Fatalf("missing blocker %q", want.id)
		}
		if got.Layer != 0 || got.Bounds != want.bounds || got.MinY != want.minY || got.MaxY != want.maxY || !got.BlocksMovement || got.BlocksLOS != want.blocksLOS || !got.Enabled {
			t.Fatalf("blocker %q=%+v, want bounds=%+v minY=%g maxY=%g blocksLOS=%t movement/enabled=true", want.id, got, want.bounds, want.minY, want.maxY, want.blocksLOS)
		}

		centerX := (want.bounds.MinX + want.bounds.MaxX) / 2
		start := world.Position{X: centerX, Y: 0, Z: want.bounds.MinZ - agent.Radius - 0.05, Layer: 0}
		if _, moveErr := nav.ResolveMove(start, world.Vec3{Z: 0.2}, agent); !errors.Is(moveErr, ErrBlocked) {
			t.Fatalf("blocker %q movement err=%v, want ErrBlocked", want.id, moveErr)
		}

		centerZ := (want.bounds.MinZ + want.bounds.MaxZ) / 2
		losY := (want.minY + want.maxY) / 2
		losFrom := world.Position{X: want.bounds.MinX - 1, Y: losY, Z: centerZ, Layer: 0}
		losTo := world.Position{X: want.bounds.MaxX + 1, Y: losY, Z: centerZ, Layer: 0}
		if gotLOS := nav.HasLineOfSight(losFrom, losTo); gotLOS == want.blocksLOS {
			t.Fatalf("blocker %q LOS=%t, blocks_los=%t", want.id, gotLOS, want.blocksLOS)
		}
	}
}
