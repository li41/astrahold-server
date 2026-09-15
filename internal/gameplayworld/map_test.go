package gameplayworld

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCastleSandboxRegistersAllWorldMasterUndergroundMaps(t *testing.T) {
	loaded, err := LoadFile(filepath.Join("..", "..", "worlds", "castle-sandbox", "gameplay.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	d := loaded.Definition
	wantOrder := []MapID{
		"map_redsoil_ant_nest",
		"map_astrahold_gaol",
		"map_ironroot_mine",
		"map_riftstone_caverns",
		"map_hollow_crypt",
		"map_sunken_abbey",
		"map_blackridge_depths",
		"map_starfall_ruins",
		"map_deep_vault",
	}
	if len(d.Maps) != len(wantOrder) {
		t.Fatalf("maps = %d, want %d", len(d.Maps), len(wantOrder))
	}
	for i, id := range wantOrder {
		if d.Maps[i].ID != id {
			t.Fatalf("map[%d].id = %q, want %q", i, d.Maps[i].ID, id)
		}
	}

	type anchor struct{ x, z float32 }
	wantAnchors := map[MapID]anchor{
		"map_redsoil_ant_nest":      {520, 220},
		"map_astrahold_gaol":        {-280, 1160},
		"map_ironroot_mine":         {-300, 2350},
		"map_riftstone_caverns":     {-980, 2480},
		"map_hollow_crypt":          {-1500, 2150},
		"map_sunken_abbey":          {-950, -520},
		"map_blackridge_depths":     {-120, 3020},
		"map_starfall_ruins":        {1950, 2700},
		"map_deep_vault":            {2200, 3200},
	}
	for id, want := range wantAnchors {
		got, ok := d.Map(id)
		if !ok {
			t.Fatalf("Map(%q) missing", id)
		}
		if got.Kind != MapKindUnderground || got.Entrance == nil || got.Entrance.Layer != 0 || got.Entrance.Point.X != want.x || got.Entrance.Point.Z != want.z {
			t.Fatalf("Map(%q) entrance = %+v, want layer=0 point=(%g,%g)", id, got.Entrance, want.x, want.z)
		}
	}

	assertExactMap(t, d, "map_astrahold_gaol", MapScaleMedium, 160, 120, 3, 32, 650)
	assertExactMap(t, d, "map_redsoil_ant_nest", MapScaleSmall, 260, 220, 3, 38, 900)
	assertExactMap(t, d, "map_riftstone_caverns", MapScaleMedium, 360, 280, 4, 72, 1450)

	for _, id := range []MapID{"map_ironroot_mine", "map_hollow_crypt"} {
		assertPlanningMap(t, d, id, MapScaleMedium, 160, 400)
	}
	for _, id := range []MapID{"map_sunken_abbey", "map_blackridge_depths", "map_starfall_ruins", "map_deep_vault"} {
		assertPlanningMap(t, d, id, MapScaleLarge, 400, 800)
	}
}

func TestValidateRejectsUndergroundMapWithoutEntrance(t *testing.T) {
	d := minimalRegionTestDefinition()
	d.Maps = []MapAuthority{{ID: "missing-entrance", Kind: MapKindUnderground, ScaleClass: MapScaleSmall, PlanningRange: &MapPlanningRange{MinMeters: 120, MaxMeters: 280}}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsMapEntranceOutsideSurface(t *testing.T) {
	d := minimalRegionTestDefinition()
	d.Maps = []MapAuthority{{ID: "outside", Kind: MapKindUnderground, ScaleClass: MapScaleSmall, PlanningRange: &MapPlanningRange{MinMeters: 120, MaxMeters: 280}, Entrance: &MapEntrance{ID: "outside-entry", Layer: 0, Point: PointXZ{X: 20, Z: 20}}}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsAmbiguousMapEnvelope(t *testing.T) {
	d := minimalRegionTestDefinition()
	d.Maps = []MapAuthority{{ID: "ambiguous", Kind: MapKindUnderground, ScaleClass: MapScaleMedium, Envelope: &MapEnvelope{WidthX: 160, LengthZ: 120}, PlanningRange: &MapPlanningRange{MinMeters: 160, MaxMeters: 400}, Entrance: &MapEntrance{ID: "entry", Layer: 0, Point: PointXZ{X: 0, Z: 0}}}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func assertExactMap(t *testing.T, d Definition, id MapID, scale MapScaleClass, widthX, lengthZ float32, levels uint16, maxDepth, route float32) {
	t.Helper()
	got, ok := d.Map(id)
	if !ok {
		t.Fatalf("Map(%q) missing", id)
	}
	if got.ScaleClass != scale || got.Envelope == nil || got.PlanningRange != nil || got.Envelope.WidthX != widthX || got.Envelope.LengthZ != lengthZ || got.LevelCount != levels || got.MaxDepthMeters != maxDepth || got.MainRouteMeters != route {
		t.Fatalf("Map(%q) = %+v, want exact %gx%g levels=%d depth=%g route=%g scale=%q", id, got, widthX, lengthZ, levels, maxDepth, route, scale)
	}
}

func assertPlanningMap(t *testing.T, d Definition, id MapID, scale MapScaleClass, minMeters, maxMeters float32) {
	t.Helper()
	got, ok := d.Map(id)
	if !ok {
		t.Fatalf("Map(%q) missing", id)
	}
	if got.ScaleClass != scale || got.Envelope != nil || got.PlanningRange == nil || got.PlanningRange.MinMeters != minMeters || got.PlanningRange.MaxMeters != maxMeters {
		t.Fatalf("Map(%q) = %+v, want planning scale=%q range=%g..%g", id, got, scale, minMeters, maxMeters)
	}
}
