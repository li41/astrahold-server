package gameplayworld

import (
	"errors"
	"strings"
	"testing"
)

const validWorldJSON = `{
  "schema_version": 5,
  "world_id": "test-world",
  "revision": "r1",
  "units": "meters",
  "agent": {"radius":0.35,"height":1.8,"max_step_height":0.5},
  "surfaces": [{
    "id":"ground","layer":0,
    "bounds":{"min_x":-10,"max_x":10,"min_z":-10,"max_z":10},
    "plane":{"origin_x":0,"origin_z":0,"base_y":0,"slope_x":0,"slope_z":0}
  }],
  "heightfields": [],
  "regions": [],
  "maps": [],
  "portals": [],
  "blockers": [],
  "gates": []
}`

func TestLoadValidWorld(t *testing.T) {
	loaded, err := Load(strings.NewReader(validWorldJSON))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Definition.WorldID != "test-world" || loaded.Definition.Revision != "r1" {
		t.Fatalf("unexpected definition: %+v", loaded.Definition)
	}
	if len(loaded.SHA256) != 64 {
		t.Fatalf("SHA256 length = %d, want 64", len(loaded.SHA256))
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	bad := strings.Replace(validWorldJSON, `"revision": "r1",`, `"revision": "r1", "legacy_grid": true,`, 1)
	_, err := Load(strings.NewReader(bad))
	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Load() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsOverlappingSameLayerSurfaces(t *testing.T) {
	loaded, err := Load(strings.NewReader(validWorldJSON))
	if err != nil {
		t.Fatal(err)
	}
	d := loaded.Definition
	second := d.Surfaces[0]
	second.ID = "ground-2"
	second.Bounds.MinX = 0
	d.Surfaces = append(d.Surfaces, second)
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsGateWithMissingBlocker(t *testing.T) {
	loaded, err := Load(strings.NewReader(validWorldJSON))
	if err != nil {
		t.Fatal(err)
	}
	d := loaded.Definition
	d.Gates = []Gate{{
		ID: "gate", BlockerID: "missing", MaxHP: 100,
		Attack: GateAttackProfile{Range: 4, Damage: 10, CooldownSeconds: 0.5},
	}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateHeightfieldBinding(t *testing.T) {
	loaded, err := Load(strings.NewReader(validWorldJSON))
	if err != nil {
		t.Fatal(err)
	}
	d := loaded.Definition
	d.Heightfields = []HeightfieldBinding{{
		SurfaceID: "ground",
		Manifest: "terrain/map1/terrain.json",
		ShellID: "land",
		DataSHA256: "eff137f190c02355ba775fa4ac3df9c958b87640af1c770e1fccc25718f678f0",
	}}
	if err := Validate(d); err != nil {
		t.Fatalf("valid heightfield binding: %v", err)
	}

	bad := d
	bad.Heightfields = append([]HeightfieldBinding(nil), d.Heightfields...)
	bad.Heightfields[0].Manifest = "../assets/land.json"
	if err := Validate(bad); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unsafe manifest err=%v", err)
	}

	bad = d
	bad.Heightfields = append([]HeightfieldBinding(nil), d.Heightfields...)
	bad.Heightfields[0].DataSHA256 = "EFF137"
	if err := Validate(bad); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("bad hash err=%v", err)
	}
}
