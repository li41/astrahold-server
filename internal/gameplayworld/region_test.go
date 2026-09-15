package gameplayworld

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCastleSandboxFirstContinentLayout(t *testing.T) {
	loaded, err := LoadFile(filepath.Join("..", "..", "worlds", "castle-sandbox", "gameplay.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	d := loaded.Definition
	if d.SchemaVersion != 4 {
		t.Fatalf("schema version = %d, want 4", d.SchemaVersion)
	}
	if len(d.Surfaces) != 1 {
		t.Fatalf("surfaces = %d, want 1", len(d.Surfaces))
	}
	wantBounds := BoundsXZ{MinX: -2600, MaxX: 2600, MinZ: -900, MaxZ: 4100}
	if d.Surfaces[0].Bounds != wantBounds {
		t.Fatalf("ground bounds = %+v, want %+v", d.Surfaces[0].Bounds, wantBounds)
	}

	expectedOrder := []RegionID{
		"region_emberwatch", "region_whisperwood", "region_thornfield", "region_astrahold",
		"region_stoneford", "region_greyfen_marsh", "region_ashen_plains", "region_raven_hollow",
		"region_broken_hills", "region_blackridge_highlands", "region_frostmere", "region_starfall_wastes",
	}
	expected := map[RegionID][]PointXZ{
		"region_emberwatch": {{X: -420, Z: -680}, {X: 180, Z: -720}, {X: 700, Z: -600}, {X: 920, Z: -350}, {X: 900, Z: -60}, {X: 720, Z: 170}, {X: 420, Z: 300}, {X: 120, Z: 320}, {X: -220, Z: 220}, {X: -480, Z: -60}},
		"region_whisperwood": {{X: 420, Z: 180}, {X: 820, Z: 120}, {X: 1200, Z: 260}, {X: 1380, Z: 520}, {X: 1320, Z: 820}, {X: 1080, Z: 950}, {X: 720, Z: 900}, {X: 500, Z: 700}, {X: 360, Z: 450}},
		"region_thornfield": {{X: 600, Z: 700}, {X: 980, Z: 620}, {X: 1380, Z: 720}, {X: 1520, Z: 980}, {X: 1420, Z: 1260}, {X: 1100, Z: 1340}, {X: 760, Z: 1240}, {X: 580, Z: 1000}},
		"region_astrahold": {{X: -780, Z: 650}, {X: -450, Z: 560}, {X: -40, Z: 580}, {X: 350, Z: 720}, {X: 520, Z: 1000}, {X: 480, Z: 1380}, {X: 120, Z: 1600}, {X: -300, Z: 1620}, {X: -650, Z: 1400}, {X: -820, Z: 1000}},
		"region_stoneford": {{X: -1550, Z: 650}, {X: -1260, Z: 580}, {X: -920, Z: 620}, {X: -620, Z: 760}, {X: -560, Z: 1080}, {X: -700, Z: 1400}, {X: -1060, Z: 1510}, {X: -1420, Z: 1340}, {X: -1580, Z: 980}},
		"region_greyfen_marsh": {{X: -1840, Z: -820}, {X: -1380, Z: -880}, {X: -920, Z: -800}, {X: -560, Z: -600}, {X: -420, Z: -280}, {X: -520, Z: 80}, {X: -900, Z: 240}, {X: -1400, Z: 200}, {X: -1820, Z: -100}},
		"region_ashen_plains": {{X: -2580, Z: -80}, {X: -2180, Z: -120}, {X: -1700, Z: -40}, {X: -1320, Z: 180}, {X: -1260, Z: 520}, {X: -1420, Z: 880}, {X: -1760, Z: 1080}, {X: -2200, Z: 1040}, {X: -2520, Z: 700}},
		"region_raven_hollow": {{X: -1880, Z: 1600}, {X: -1580, Z: 1520}, {X: -1280, Z: 1600}, {X: -1100, Z: 1840}, {X: -1120, Z: 2180}, {X: -1340, Z: 2380}, {X: -1680, Z: 2360}, {X: -1900, Z: 2140}},
		"region_broken_hills": {{X: -1380, Z: 1620}, {X: -980, Z: 1540}, {X: -580, Z: 1600}, {X: -180, Z: 1800}, {X: 80, Z: 2140}, {X: 20, Z: 2520}, {X: -320, Z: 2760}, {X: -760, Z: 2800}, {X: -1120, Z: 2600}, {X: -1420, Z: 2200}},
		"region_blackridge_highlands": {{X: -1080, Z: 2680}, {X: -700, Z: 2600}, {X: -260, Z: 2640}, {X: 200, Z: 2780}, {X: 560, Z: 3060}, {X: 620, Z: 3460}, {X: 400, Z: 3820}, {X: 40, Z: 3980}, {X: -400, Z: 4000}, {X: -800, Z: 3860}, {X: -1060, Z: 3540}, {X: -1100, Z: 3100}},
		"region_frostmere": {{X: -820, Z: 3320}, {X: -520, Z: 3220}, {X: -180, Z: 3280}, {X: 120, Z: 3480}, {X: 140, Z: 3820}, {X: -80, Z: 4040}, {X: -460, Z: 4080}, {X: -780, Z: 3900}, {X: -880, Z: 3600}},
		"region_starfall_wastes": {{X: 760, Z: 1620}, {X: 1120, Z: 1500}, {X: 1580, Z: 1500}, {X: 2040, Z: 1660}, {X: 2440, Z: 2000}, {X: 2520, Z: 2420}, {X: 2300, Z: 2840}, {X: 1900, Z: 2940}, {X: 1460, Z: 2860}, {X: 1080, Z: 2660}, {X: 820, Z: 2300}},
	}

	if len(d.Regions) != len(expectedOrder) {
		t.Fatalf("regions = %d, want %d", len(d.Regions), len(expectedOrder))
	}
	for i, region := range d.Regions {
		if region.ID != expectedOrder[i] {
			t.Fatalf("region[%d].id = %q, want %q", i, region.ID, expectedOrder[i])
		}
		if !reflect.DeepEqual(region.Polygon, expected[region.ID]) {
			t.Fatalf("region %q polygon = %+v, want %+v", region.ID, region.Polygon, expected[region.ID])
		}
	}
	if d.Regions[10].ParentID != "region_blackridge_highlands" {
		t.Fatalf("Frostmere parent = %q, want region_blackridge_highlands", d.Regions[10].ParentID)
	}

	atFrostmere := d.RegionsAt(0, -350, 3650)
	got := make(map[RegionID]bool, len(atFrostmere))
	for _, region := range atFrostmere {
		got[region.ID] = true
	}
	if !got["region_frostmere"] || !got["region_blackridge_highlands"] {
		t.Fatalf("RegionsAt(Frostmere) = %+v, want child and parent", got)
	}
}

func TestValidateRejectsRegionOutsideSurface(t *testing.T) {
	d := minimalRegionTestDefinition()
	d.Regions = []RegionCore{{ID: "outside", Layer: 0, Polygon: []PointXZ{{X: -2, Z: -2}, {X: 2, Z: -2}, {X: 20, Z: 2}}}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsRegionParentCycle(t *testing.T) {
	d := minimalRegionTestDefinition()
	triangle := []PointXZ{{X: -2, Z: -2}, {X: 2, Z: -2}, {X: 0, Z: 2}}
	d.Regions = []RegionCore{{ID: "a", ParentID: "b", Layer: 0, Polygon: triangle}, {ID: "b", ParentID: "a", Layer: 0, Polygon: triangle}}
	if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDefinition", err)
	}
}

func minimalRegionTestDefinition() Definition {
	return Definition{SchemaVersion: SchemaVersion, WorldID: "region-test", Revision: "r1", Units: "meters", Agent: AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5}, Surfaces: []Surface{{ID: "ground", Layer: 0, Bounds: BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}, Plane: SurfacePlane{}}}}
}
