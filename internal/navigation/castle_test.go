package navigation

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCastleSandboxGateAndStairTraversal(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil { t.Fatal(err) }
	nav, err := NewGameplayNavigator(loaded.Definition)
	if err != nil { t.Fatal(err) }
	agent := Agent{Radius: loaded.Definition.Agent.Radius, MaxStepHeight: loaded.Definition.Agent.MaxStepHeight}

	outside := world.Position{X: 0, Y: 0, Z: 8, Layer: 0}
	if _, err := nav.ResolveMove(outside, world.Vec3{Z: 3}, agent); !errors.Is(err, ErrBlocked) {
		t.Fatalf("closed gate move error = %v, want ErrBlocked", err)
	}
	from := world.Position{X: 0, Y: 1, Z: 6, Layer: 0}
	insideTarget := world.Position{X: 0, Y: 1, Z: 15, Layer: 0}
	if nav.HasLineOfSight(from, insideTarget) {
		t.Fatal("closed gate LOS = true, want false")
	}
	gateTarget := world.Position{X: 0, Y: 1, Z: 9.5, Layer: 0}
	if !nav.HasLineOfSightIgnoringBlocker(from, gateTarget, "main-gate") {
		t.Fatal("gate interaction LOS = false, target gate should not occlude itself")
	}

	if err := nav.SetBlockerEnabled("main-gate", false); err != nil { t.Fatal(err) }
	inside, err := nav.ResolveMove(outside, world.Vec3{Z: 3}, agent)
	if err != nil { t.Fatalf("open gate move error = %v", err) }
	if inside.Layer != 0 || inside.Z != 11 { t.Fatalf("open gate position = %+v", inside) }
	if !nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 0, Layer: 0}, insideTarget) {
		t.Fatal("open gate LOS = false, want true")
	}

	pos := world.Position{X: -25, Y: 0, Z: 20.4, Layer: 0}
	pos, err = nav.ResolveMove(pos, world.Vec3{Z: -0.6}, agent)
	if err != nil { t.Fatalf("ground -> stair error = %v", err) }
	if pos.Layer != 1 || pos.Y < 0.19 || pos.Y > 0.21 {
		t.Fatalf("ground -> stair position = %+v", pos)
	}

	for i := 0; i < 30 && pos.Layer != 2; i++ {
		pos, err = nav.ResolveMove(pos, world.Vec3{Z: -0.3}, agent)
		if err != nil { t.Fatalf("stair traversal step=%d pos=%+v err=%v", i, pos, err) }
	}
	if pos.Layer != 2 || pos.Y != 8 {
		t.Fatalf("stair -> wall position = %+v, want layer=2 y=8", pos)
	}

	for i := 0; i < 3; i++ {
		pos, err = nav.ResolveMove(pos, world.Vec3{Z: -0.3}, agent)
		if err != nil { t.Fatalf("walk onto wall step=%d: %v", i, err) }
	}
	for i := 0; i < 6 && pos.Layer != 1; i++ {
		pos, err = nav.ResolveMove(pos, world.Vec3{Z: 0.3}, agent)
		if err != nil { t.Fatalf("wall -> stair step=%d pos=%+v err=%v", i, pos, err) }
	}
	if pos.Layer != 1 || pos.Y >= 8 {
		t.Fatalf("wall -> stair position = %+v", pos)
	}
}

func TestCastleSandboxEmberwatchVillageBlockers(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Revision != "s5b-commercial-mpv-002" {
		t.Fatalf("revision=%q, want s5b-commercial-mpv-002", loaded.Definition.Revision)
	}

	type expectedBlocker struct {
		id        string
		bounds    gameplayworld.BoundsXZ
		maxY      float32
		blocksLOS bool
	}
	expected := []expectedBlocker{
		{id: "emberwatch-warden-post", bounds: gameplayworld.BoundsXZ{MinX: -11.8, MaxX: -9.2, MinZ: -43.8, MaxZ: -33.2}, maxY: 4.6, blocksLOS: true},
		{id: "emberwatch-house-west", bounds: gameplayworld.BoundsXZ{MinX: -29.3, MaxX: -20.7, MinZ: -27.6, MaxZ: -23.4}, maxY: 5.2, blocksLOS: true},
		{id: "emberwatch-cottage-west", bounds: gameplayworld.BoundsXZ{MinX: -16.8, MaxX: -14.2, MinZ: -18, MaxZ: -13}, maxY: 4.6, blocksLOS: true},
		{id: "emberwatch-house-east", bounds: gameplayworld.BoundsXZ{MinX: 21.1, MaxX: 26.9, MinZ: -16.2, MaxZ: -7.8}, maxY: 5.2, blocksLOS: true},
		{id: "emberwatch-gate-hut", bounds: gameplayworld.BoundsXZ{MinX: 6.7, MaxX: 10.3, MinZ: -8.6, MaxZ: 1.6}, maxY: 4.6, blocksLOS: true},
		{id: "emberwatch-cottage-east", bounds: gameplayworld.BoundsXZ{MinX: 14.6, MaxX: 18.4, MinZ: -34, MaxZ: -31}, maxY: 4.6, blocksLOS: true},
		{id: "emberwatch-chapel", bounds: gameplayworld.BoundsXZ{MinX: -23.4, MaxX: -9.6, MinZ: -7.7, MaxZ: 1.7}, maxY: 12.1, blocksLOS: true},
		{id: "emberwatch-market-shed", bounds: gameplayworld.BoundsXZ{MinX: 4.6, MaxX: 13.4, MinZ: -38.5, MaxZ: -36.5}, maxY: 3.8, blocksLOS: true},
		{id: "emberwatch-well", bounds: gameplayworld.BoundsXZ{MinX: 4.6, MaxX: 7.4, MinZ: -34.4, MaxZ: -31.6}, maxY: 3, blocksLOS: false},
		{id: "emberwatch-campfire", bounds: gameplayworld.BoundsXZ{MinX: -5.4, MaxX: -3.6, MinZ: -41.4, MaxZ: -39.6}, maxY: 1, blocksLOS: false},
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
		if got.Layer != 0 || got.Bounds != want.bounds || got.MinY != 0 || got.MaxY != want.maxY || !got.BlocksMovement || got.BlocksLOS != want.blocksLOS || !got.Enabled {
			t.Fatalf("blocker %q=%+v, want bounds=%+v maxY=%g blocksLOS=%t movement/enabled=true", want.id, got, want.bounds, want.maxY, want.blocksLOS)
		}

		centerX := (want.bounds.MinX + want.bounds.MaxX) / 2
		start := world.Position{X: centerX, Y: 0, Z: want.bounds.MinZ - agent.Radius - 0.05, Layer: 0}
		if _, moveErr := nav.ResolveMove(start, world.Vec3{Z: 0.2}, agent); !errors.Is(moveErr, ErrBlocked) {
			t.Fatalf("blocker %q movement err=%v, want ErrBlocked", want.id, moveErr)
		}

		centerZ := (want.bounds.MinZ + want.bounds.MaxZ) / 2
		losFrom := world.Position{X: want.bounds.MinX - 1, Y: 0.9, Z: centerZ, Layer: 0}
		losTo := world.Position{X: want.bounds.MaxX + 1, Y: 0.9, Z: centerZ, Layer: 0}
		if gotLOS := nav.HasLineOfSight(losFrom, losTo); gotLOS == want.blocksLOS {
			t.Fatalf("blocker %q LOS=%t, blocks_los=%t", want.id, gotLOS, want.blocksLOS)
		}
	}
}
