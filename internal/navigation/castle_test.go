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
	if nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 0, Layer: 0}, world.Position{X: 0, Y: 1, Z: 15, Layer: 0}) {
		t.Fatal("closed gate LOS = true, want false")
	}

	if err := nav.SetBlockerEnabled("main-gate", false); err != nil { t.Fatal(err) }
	inside, err := nav.ResolveMove(outside, world.Vec3{Z: 3}, agent)
	if err != nil { t.Fatalf("open gate move error = %v", err) }
	if inside.Layer != 0 || inside.Z != 11 { t.Fatalf("open gate position = %+v", inside) }
	if !nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 0, Layer: 0}, world.Position{X: 0, Y: 1, Z: 15, Layer: 0}) {
		t.Fatal("open gate LOS = false, want true")
	}

	// 從內庭進入西側斜坡。
	pos := world.Position{X: -25, Y: 0, Z: 20.4, Layer: 0}
	pos, err = nav.ResolveMove(pos, world.Vec3{Z: -0.6}, agent)
	if err != nil { t.Fatalf("ground -> stair error = %v", err) }
	if pos.Layer != 1 || pos.Y < 0.19 || pos.Y > 0.21 {
		t.Fatalf("ground -> stair position = %+v", pos)
	}

	// 沿斜坡上行；跨入 top portal 時應依同一接點的 surface gap 判斷，而不是整個 tick 的 from.Y。
	for i := 0; i < 30 && pos.Layer != 2; i++ {
		pos, err = nav.ResolveMove(pos, world.Vec3{Z: -0.3}, agent)
		if err != nil { t.Fatalf("stair traversal step=%d pos=%+v err=%v", i, pos, err) }
	}
	if pos.Layer != 2 || pos.Y != 8 {
		t.Fatalf("stair -> wall position = %+v, want layer=2 y=8", pos)
	}

	// 先走離 portal，再反向進入，驗證 bidirectional transition。
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
