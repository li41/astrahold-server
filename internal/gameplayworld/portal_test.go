package gameplayworld

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestCurrentStarterWorldIsValid(t *testing.T) {
	loaded, err := LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatalf("LoadFile(castle-sandbox) error = %v", err)
	}
	if loaded.Definition.Revision != "first-continent-world-master-v2" {
		t.Fatalf("revision = %q, want first-continent-world-master-v2", loaded.Definition.Revision)
	}
	if len(loaded.Definition.Surfaces) != 1 || len(loaded.Definition.Regions) != 12 || len(loaded.Definition.Maps) != 9 || len(loaded.Definition.Portals) != 0 || len(loaded.Definition.Blockers) != 10 || len(loaded.Definition.Gates) != 0 {
		t.Fatalf("unexpected starter world topology: surfaces=%d regions=%d maps=%d portals=%d blockers=%d gates=%d", len(loaded.Definition.Surfaces), len(loaded.Definition.Regions), len(loaded.Definition.Maps), len(loaded.Definition.Portals), len(loaded.Definition.Blockers), len(loaded.Definition.Gates))
	}
	ground := loaded.Definition.Surfaces[0]
	if ground.ID != "ground" || ground.Layer != 0 {
		t.Fatalf("unexpected ground surface: %+v", ground)
	}
}

func TestValidateRejectsPortalHeightGapAboveMaxStep(t *testing.T) {
	d := Definition{
		SchemaVersion: SchemaVersion,
		WorldID:       "bad-portal",
		Revision:      "r1",
		Units:         "meters",
		Agent:         AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []Surface{
			{ID: "ground", Layer: 0, Bounds: BoundsXZ{MinX: -2, MaxX: 2, MinZ: -2, MaxZ: 2}, Plane: SurfacePlane{}},
			{ID: "upper", Layer: 1, Bounds: BoundsXZ{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1}, Plane: SurfacePlane{BaseY: 1}},
		},
		Portals: []Portal{{
			ID: "too-high", FromLayer: 0, ToLayer: 1,
			Bounds:        BoundsXZ{MinX: -0.5, MaxX: 0.5, MinZ: 0, MaxZ: 0.2},
			Bidirectional: true,
		}},
	}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsPortalOutsideTargetSurface(t *testing.T) {
	d := Definition{
		SchemaVersion: SchemaVersion,
		WorldID:       "bad-trigger",
		Revision:      "r1",
		Units:         "meters",
		Agent:         AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []Surface{
			{ID: "ground", Layer: 0, Bounds: BoundsXZ{MinX: -2, MaxX: 2, MinZ: -2, MaxZ: 2}, Plane: SurfacePlane{}},
			{ID: "upper", Layer: world.LayerID(1), Bounds: BoundsXZ{MinX: 0, MaxX: 1, MinZ: 0, MaxZ: 1}, Plane: SurfacePlane{}},
		},
		Portals: []Portal{{
			ID: "outside", FromLayer: 0, ToLayer: 1,
			Bounds: BoundsXZ{MinX: -0.5, MaxX: 0.5, MinZ: 0, MaxZ: 0.5},
		}},
	}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}
