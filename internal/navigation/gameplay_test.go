package navigation

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestGameplayNavigatorDynamicGateAndLOS(t *testing.T) {
	nav := mustTestNavigator(t)
	agent := Agent{Radius: 0.35, MaxStepHeight: 0.5}
	start := world.Position{X: 0, Y: 0, Z: 0, Layer: 0}

	if _, err := nav.ResolveMove(start, world.Vec3{Z: 3}, agent); !errors.Is(err, ErrBlocked) {
		t.Fatalf("gate enabled ResolveMove() error = %v, want ErrBlocked", err)
	}
	if nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 0, Layer: 0}, world.Position{X: 0, Y: 1, Z: 3, Layer: 0}) {
		t.Fatal("gate enabled LOS = true, want false")
	}

	if err := nav.SetBlockerEnabled("main-gate", false); err != nil {
		t.Fatal(err)
	}
	next, err := nav.ResolveMove(start, world.Vec3{Z: 3}, agent)
	if err != nil {
		t.Fatalf("gate disabled ResolveMove() error = %v", err)
	}
	if next.Z != 3 || next.Layer != 0 {
		t.Fatalf("unexpected next position: %+v", next)
	}
	if !nav.HasLineOfSight(world.Position{X: 0, Y: 1, Z: 0, Layer: 0}, world.Position{X: 0, Y: 1, Z: 3, Layer: 0}) {
		t.Fatal("gate disabled LOS = false, want true")
	}
	if err := nav.SetBlockerEnabled("missing", false); !errors.Is(err, ErrUnknownBlocker) {
		t.Fatalf("unknown blocker error = %v", err)
	}
}

func TestGameplayNavigatorRampAndLayerTransition(t *testing.T) {
	nav := mustTestNavigator(t)
	agent := Agent{Radius: 0.2, MaxStepHeight: 0.5}
	pos := world.Position{X: 3, Y: 0, Z: -0.1, Layer: 0}

	var err error
	pos, err = nav.ResolveMove(pos, world.Vec3{Z: 0.2}, agent)
	if err != nil {
		t.Fatalf("ground->ramp error = %v", err)
	}
	if pos.Layer != 1 || pos.Y < 0.09 || pos.Y > 0.11 {
		t.Fatalf("ground->ramp position = %+v", pos)
	}

	for pos.Z < 3.7 {
		pos, err = nav.ResolveMove(pos, world.Vec3{Z: 0.3}, agent)
		if err != nil {
			t.Fatalf("ramp step at z=%.2f error = %v", pos.Z, err)
		}
	}
	pos, err = nav.ResolveMove(pos, world.Vec3{Z: 0.25}, agent)
	if err != nil {
		t.Fatalf("ramp->wall error = %v", err)
	}
	if pos.Layer != 2 || pos.Y != 4 {
		t.Fatalf("ramp->wall position = %+v", pos)
	}
}

func mustTestNavigator(t *testing.T) *GameplayNavigator {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "nav-test",
		Revision:      "r1",
		Units:         "meters",
		Agent: gameplayworld.AgentDefaults{
			Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5,
		},
		Surfaces: []gameplayworld.Surface{
			{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}, Plane: gameplayworld.SurfacePlane{}},
			{ID: "ramp", Layer: 1, Bounds: gameplayworld.BoundsXZ{MinX: 2, MaxX: 4, MinZ: 0, MaxZ: 4}, Plane: gameplayworld.SurfacePlane{OriginX: 3, OriginZ: 0, BaseY: 0, SlopeZ: 1}},
			{ID: "wall", Layer: 2, Bounds: gameplayworld.BoundsXZ{MinX: 2, MaxX: 4, MinZ: 3.9, MaxZ: 6}, Plane: gameplayworld.SurfacePlane{BaseY: 4}},
		},
		Portals: []gameplayworld.Portal{
			{ID: "ground-ramp", FromLayer: 0, ToLayer: 1, Bounds: gameplayworld.BoundsXZ{MinX: 2, MaxX: 4, MinZ: 0, MaxZ: 0.2}, Bidirectional: true},
			{ID: "ramp-wall", FromLayer: 1, ToLayer: 2, Bounds: gameplayworld.BoundsXZ{MinX: 2, MaxX: 4, MinZ: 3.9, MaxZ: 4}, Bidirectional: true},
		},
		Blockers: []gameplayworld.Blocker{
			{ID: "main-gate", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2}, MinY: 0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true},
		},
	}
	nav, err := NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	return nav
}
