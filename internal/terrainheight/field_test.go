package terrainheight

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHeightAtUsesBilinearWorldSpaceSampling(t *testing.T) {
	field, err := New(Definition{
		ID: "land", SamplesX: 3, SamplesZ: 2,
		SizeX: 2, SizeZ: 1, OriginX: 0, OriginZ: 0,
		StepX: 1, StepZ: 1,
	}, []float32{
		0, 10, 20,
		10, 20, 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := field.HeightAt(0, 0); !ok || math.Abs(float64(got-15)) > 0.0001 {
		t.Fatalf("HeightAt(0,0)=(%v,%v) want (15,true)", got, ok)
	}
	if got, ok := field.HeightAt(-1, -0.5); !ok || got != 0 {
		t.Fatalf("HeightAt(min)=(%v,%v) want (0,true)", got, ok)
	}
	if got, ok := field.HeightAt(1, 0.5); !ok || got != 30 {
		t.Fatalf("HeightAt(max)=(%v,%v) want (30,true)", got, ok)
	}
	if _, ok := field.HeightAt(1.01, 0); ok {
		t.Fatal("out-of-bounds sample unexpectedly resolved")
	}
}

func TestLoadFileValidatesPublishedF32LEContractAndHashes(t *testing.T) {
	dir := t.TempDir()
	values := []float32{0, 10, 20, 10, 20, 30}
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:i*4+4], math.Float32bits(value))
	}
	digest := sha256.Sum256(data)
	dataSHA := hex.EncodeToString(digest[:])
	if err := os.WriteFile(filepath.Join(dir, "land.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
  "name":"map1",
  "seed":1108,
  "intent":"ignored presentation/build metadata is allowed",
  "format":"f32le",
  "shells":[{
    "id":"land",
    "file":"land.bin",
    "samples":[3,2],
    "size":[2,1],
    "stepXZ":[1,1],
    "origin":{"x":0,"z":0},
    "minHeight":0,
    "maxHeight":30,
    "sha256":"` + dataSHA + `"
  }],
  "water":{"level":0}
}`)
	manifestPath := filepath.Join(dir, "terrain.json")
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(manifestPath, "land")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Field == nil || loaded.DataSHA256 != dataSHA || len(loaded.ManifestSHA256) != 64 || len(loaded.SHA256) != 64 {
		t.Fatalf("loaded=%+v", loaded)
	}
	if got, ok := loaded.Field.HeightAt(0, 0); !ok || got != 15 {
		t.Fatalf("loaded HeightAt=(%v,%v), want (15,true)", got, ok)
	}
}

func TestLoadFileRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 2*2*4)
	if err := os.WriteFile(filepath.Join(dir, "land.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
  "format":"f32le",
  "shells":[{
    "id":"land","file":"land.bin","samples":[2,2],"size":[1,1],"stepXZ":[1,1],
    "origin":{"x":0,"z":0},"minHeight":0,"maxHeight":0,
    "sha256":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
  }]
}`)
	path := filepath.Join(dir, "terrain.json")
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, "land"); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("err=%v want ErrHashMismatch", err)
	}
}

func TestLoadFileRejectsEscapingDataPath(t *testing.T) {
	dir := t.TempDir()
	manifest := []byte(`{
  "format":"f32le",
  "shells":[{
    "id":"land","file":"../land.bin","samples":[2,2],"size":[1,1],"stepXZ":[1,1],
    "origin":{"x":0,"z":0},"minHeight":0,"maxHeight":0,
    "sha256":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
  }]
}`)
	path := filepath.Join(dir, "terrain.json")
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, "land"); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("err=%v want ErrInvalidManifest", err)
	}
}

func TestNewRejectsNonFiniteAndBadStep(t *testing.T) {
	if _, err := New(Definition{
		ID: "land", SamplesX: 2, SamplesZ: 2, SizeX: 1, SizeZ: 1,
		OriginX: 0, OriginZ: 0, StepX: 2, StepZ: 1,
	}, []float32{0, 0, 0, 0}); !errors.Is(err, ErrInvalidField) {
		t.Fatalf("bad step err=%v", err)
	}
	if _, err := New(Definition{
		ID: "land", SamplesX: 2, SamplesZ: 2, SizeX: 1, SizeZ: 1,
		OriginX: 0, OriginZ: 0, StepX: 1, StepZ: 1,
	}, []float32{0, float32(math.Inf(1)), 0, 0}); !errors.Is(err, ErrInvalidField) {
		t.Fatalf("non-finite err=%v", err)
	}
}
