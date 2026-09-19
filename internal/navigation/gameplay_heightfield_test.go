package navigation

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/terrainheight"
	"github.com/li41/astrahold-server/internal/world"
)

func TestGameplayNavigatorHeightfieldOverridesPlaneInsideField(t *testing.T) {
	field, err := terrainheight.New(terrainheight.Definition{
		ID: "land", SamplesX: 3, SamplesZ: 3,
		SizeX: 2, SizeZ: 2, OriginX: 0, OriginZ: 0,
		StepX: 1, StepZ: 1,
	}, []float32{
		10.0, 10.0, 10.0,
		10.2, 10.2, 10.2,
		10.4, 10.4, 10.4,
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "terrain-nav", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10},
			Plane: gameplayworld.SurfacePlane{BaseY: 2},
		}},
	}
	nav, err := NewGameplayNavigatorWithHeightfields(definition, map[string]*terrainheight.Field{"ground": field})
	if err != nil {
		t.Fatal(err)
	}

	if height, ok := nav.GroundHeightAt(0, 0, 0); !ok || height != 10.2 {
		t.Fatalf("height=(%v,%v) want (10.2,true)", height, ok)
	}
	projected, err := nav.ResolveGroundPosition(world.Position{X: 0, Y: -99, Z: 0, Layer: 0})
	if err != nil || projected.Y != 10.2 {
		t.Fatalf("projected=%+v err=%v", projected, err)
	}

	// The stored Y is deliberately stale. Movement compares terrain-to-terrain height instead of
	// trapping the actor forever because an older spawn source still carried a flat-plane Y.
	next, err := nav.ResolveMove(
		world.Position{X: 0, Y: 0, Z: -0.5, Layer: 0},
		world.Vec3{Z: 0.5},
		Agent{Radius: 0.35, MaxStepHeight: 0.5},
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Y != 10.2 {
		t.Fatalf("next=%+v want terrain y=10.2", next)
	}

	// The baked field covers only -1..1. The enclosing gameplay surface remains valid outside it
	// and falls back to its authored plane.
	if height, ok := nav.GroundHeightAt(0, 5, 5); !ok || height != 2 {
		t.Fatalf("fallback height=(%v,%v) want (2,true)", height, ok)
	}
}

func TestGameplayNavigatorRejectsHeightfieldOutsideAuthoredSurface(t *testing.T) {
	field, err := terrainheight.New(terrainheight.Definition{
		ID: "land", SamplesX: 2, SamplesZ: 2,
		SizeX: 20, SizeZ: 20, OriginX: 0, OriginZ: 0,
		StepX: 20, StepZ: 20,
	}, []float32{0, 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "terrain-nav", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -5, MaxX: 5, MinZ: -5, MaxZ: 5},
			Plane: gameplayworld.SurfacePlane{},
		}},
	}
	if _, err := NewGameplayNavigatorWithHeightfields(definition, map[string]*terrainheight.Field{"ground": field}); !errors.Is(err, ErrInvalidTerrainHeightfield) {
		t.Fatalf("err=%v want ErrInvalidTerrainHeightfield", err)
	}
	if _, err := NewGameplayNavigatorWithHeightfields(definition, map[string]*terrainheight.Field{"missing": field}); !errors.Is(err, ErrInvalidTerrainHeightfield) {
		t.Fatalf("unknown surface err=%v want ErrInvalidTerrainHeightfield", err)
	}
}
