package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

func TestLoadWorldHeightfieldsBindsVerifiedServerLocalTerrain(t *testing.T) {
	dir := t.TempDir()
	terrainDir := filepath.Join(dir, "terrain", "map1")
	if err := os.MkdirAll(terrainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	values := []float32{10, 11, 12, 13}
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	digest := sha256.Sum256(data)
	dataSHA := hex.EncodeToString(digest[:])
	if err := os.WriteFile(filepath.Join(terrainDir, "land.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"name": "test", "format": "f32le",
		"shells": []any{map[string]any{
			"id": "land", "file": "land.bin", "samples": []int{2, 2},
			"size": []float32{2, 2}, "stepXZ": []float32{2, 2},
			"origin": map[string]float32{"x": 0, "z": 0},
			"minHeight": float32(10), "maxHeight": float32(13), "sha256": dataSHA,
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(terrainDir, "terrain.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "test", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1},
			Plane: gameplayworld.SurfacePlane{},
		}},
		Heightfields: []gameplayworld.HeightfieldBinding{{
			SurfaceID: "ground", Manifest: "terrain/map1/terrain.json", ShellID: "land", DataSHA256: dataSHA,
		}},
	}
	fields, err := loadWorldHeightfields(filepath.Join(dir, "gameplay.json"), definition)
	if err != nil {
		t.Fatal(err)
	}
	field := fields["ground"]
	if field == nil {
		t.Fatal("ground heightfield missing")
	}
	if height, ok := field.HeightAt(-1, -1); !ok || height != 10 {
		t.Fatalf("height=(%g,%t) want (10,true)", height, ok)
	}
}
