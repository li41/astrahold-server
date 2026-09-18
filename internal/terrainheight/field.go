// Package terrainheight loads baked Server-authoritative terrain height fields.
//
// The current Map1 art/content pipeline publishes f32le height grids in the same world-space frame
// used by the gameplay world. This package consumes only the numeric terrain contract; it does not
// know Client meshes, materials, scatter, or other presentation assets.
package terrainheight

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidManifest  = errors.New("terrainheight: invalid manifest")
	ErrInvalidField     = errors.New("terrainheight: invalid field")
	ErrUnsupportedFormat = errors.New("terrainheight: unsupported format")
	ErrShellNotFound    = errors.New("terrainheight: shell not found")
	ErrHashMismatch     = errors.New("terrainheight: data hash mismatch")
)

const FormatF32LE = "f32le"

type BoundsXZ struct {
	MinX float32
	MaxX float32
	MinZ float32
	MaxZ float32
}

func (b BoundsXZ) Contains(x, z float32) bool {
	return x >= b.MinX && x <= b.MaxX && z >= b.MinZ && z <= b.MaxZ
}

type Definition struct {
	ID       string
	SamplesX int
	SamplesZ int
	SizeX    float32
	SizeZ    float32
	OriginX  float32
	OriginZ  float32
	StepX    float32
	StepZ    float32
}

type Field struct {
	definition Definition
	bounds     BoundsXZ
	values     []float32
	minHeight float32
	maxHeight float32
}

type Loaded struct {
	Field          *Field
	ManifestSHA256 string
	DataSHA256     string
	SHA256         string
}

type terrainManifest struct {
	Name   string          `json:"name"`
	Format string          `json:"format"`
	Shells []terrainShell  `json:"shells"`
}

type terrainShell struct {
	ID        string       `json:"id"`
	File      string       `json:"file"`
	Samples   [2]int       `json:"samples"`
	Size      [2]float32   `json:"size"`
	StepXZ    [2]float32   `json:"stepXZ"`
	Origin    terrainPoint `json:"origin"`
	MinHeight float32      `json:"minHeight"`
	MaxHeight float32      `json:"maxHeight"`
	SHA256    string       `json:"sha256"`
}

type terrainPoint struct {
	X float32 `json:"x"`
	Z float32 `json:"z"`
}

func New(def Definition, values []float32) (*Field, error) {
	if strings.TrimSpace(def.ID) == "" || def.SamplesX < 2 || def.SamplesZ < 2 {
		return nil, ErrInvalidField
	}
	if !positiveFinite(def.SizeX) || !positiveFinite(def.SizeZ) || !finite(def.OriginX) || !finite(def.OriginZ) {
		return nil, ErrInvalidField
	}
	expectedStepX := def.SizeX / float32(def.SamplesX-1)
	expectedStepZ := def.SizeZ / float32(def.SamplesZ-1)
	if def.StepX == 0 {
		def.StepX = expectedStepX
	}
	if def.StepZ == 0 {
		def.StepZ = expectedStepZ
	}
	if !positiveFinite(def.StepX) || !positiveFinite(def.StepZ) || !near(def.StepX, expectedStepX, 0.0005) || !near(def.StepZ, expectedStepZ, 0.0005) {
		return nil, ErrInvalidField
	}
	count, ok := sampleCount(def.SamplesX, def.SamplesZ)
	if !ok || len(values) != count {
		return nil, ErrInvalidField
	}
	owned := append([]float32(nil), values...)
	minHeight := float32(math.MaxFloat32)
	maxHeight := -float32(math.MaxFloat32)
	for _, value := range owned {
		if !finite(value) {
			return nil, ErrInvalidField
		}
		if value < minHeight {
			minHeight = value
		}
		if value > maxHeight {
			maxHeight = value
		}
	}
	return &Field{
		definition: def,
		bounds: BoundsXZ{
			MinX: def.OriginX - def.SizeX/2,
			MaxX: def.OriginX + def.SizeX/2,
			MinZ: def.OriginZ - def.SizeZ/2,
			MaxZ: def.OriginZ + def.SizeZ/2,
		},
		values:     owned,
		minHeight: minHeight,
		maxHeight: maxHeight,
	}, nil
}

func LoadFile(manifestPath, shellID string) (Loaded, error) {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Loaded{}, err
	}
	return load(manifestBytes, filepath.Dir(manifestPath), shellID)
}

func load(manifestBytes []byte, baseDir, shellID string) (Loaded, error) {
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	var manifest terrainManifest
	if err := decoder.Decode(&manifest); err != nil {
		return Loaded{}, fmt.Errorf("%w: decode: %v", ErrInvalidManifest, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Loaded{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidManifest)
		}
		return Loaded{}, fmt.Errorf("%w: trailing data: %v", ErrInvalidManifest, err)
	}
	if manifest.Format != FormatF32LE || len(manifest.Shells) == 0 {
		if manifest.Format != FormatF32LE {
			return Loaded{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, manifest.Format)
		}
		return Loaded{}, ErrInvalidManifest
	}
	shellID = strings.TrimSpace(shellID)
	if shellID == "" {
		return Loaded{}, ErrShellNotFound
	}
	var shell *terrainShell
	for i := range manifest.Shells {
		if manifest.Shells[i].ID == shellID {
			shell = &manifest.Shells[i]
			break
		}
	}
	if shell == nil {
		return Loaded{}, fmt.Errorf("%w: %s", ErrShellNotFound, shellID)
	}
	relative, ok := safeRelativePath(shell.File)
	if !ok {
		return Loaded{}, ErrInvalidManifest
	}
	data, err := os.ReadFile(filepath.Join(baseDir, relative))
	if err != nil {
		return Loaded{}, err
	}
	dataDigest := sha256.Sum256(data)
	dataSHA := hex.EncodeToString(dataDigest[:])
	if shell.SHA256 == "" || !strings.EqualFold(shell.SHA256, dataSHA) {
		return Loaded{}, fmt.Errorf("%w: got=%s want=%s", ErrHashMismatch, dataSHA, shell.SHA256)
	}
	count, ok := sampleCount(shell.Samples[0], shell.Samples[1])
	if !ok || len(data) != count*4 {
		return Loaded{}, ErrInvalidField
	}
	values := make([]float32, count)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4 : i*4+4]))
	}
	field, err := New(Definition{
		ID: shell.ID, SamplesX: shell.Samples[0], SamplesZ: shell.Samples[1],
		SizeX: shell.Size[0], SizeZ: shell.Size[1],
		OriginX: shell.Origin.X, OriginZ: shell.Origin.Z,
		StepX: shell.StepXZ[0], StepZ: shell.StepXZ[1],
	}, values)
	if err != nil {
		return Loaded{}, err
	}
	if !finite(shell.MinHeight) || !finite(shell.MaxHeight) || shell.MinHeight > shell.MaxHeight ||
		!near(field.minHeight, shell.MinHeight, 0.001) || !near(field.maxHeight, shell.MaxHeight, 0.001) {
		return Loaded{}, ErrInvalidField
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	manifestSHA := hex.EncodeToString(manifestDigest[:])
	composite := sha256.New()
	_, _ = composite.Write([]byte("astrahold/terrainheight/v1\x00"))
	_, _ = composite.Write(manifestDigest[:])
	_, _ = composite.Write(dataDigest[:])
	return Loaded{
		Field: field, ManifestSHA256: manifestSHA, DataSHA256: dataSHA,
		SHA256: hex.EncodeToString(composite.Sum(nil)),
	}, nil
}

func (f *Field) Definition() Definition {
	if f == nil {
		return Definition{}
	}
	return f.definition
}

func (f *Field) Bounds() BoundsXZ {
	if f == nil {
		return BoundsXZ{}
	}
	return f.bounds
}

func (f *Field) MinHeight() float32 {
	if f == nil {
		return 0
	}
	return f.minHeight
}

func (f *Field) MaxHeight() float32 {
	if f == nil {
		return 0
	}
	return f.maxHeight
}

// HeightAt bilinearly samples the baked field. The field is immutable after load, so this is safe
// on the world tick and performs no I/O or allocation.
func (f *Field) HeightAt(x, z float32) (float32, bool) {
	if f == nil || !f.bounds.Contains(x, z) {
		return 0, false
	}
	def := f.definition
	fx := (x - f.bounds.MinX) / def.StepX
	fz := (z - f.bounds.MinZ) / def.StepZ
	maxX := float32(def.SamplesX - 1)
	maxZ := float32(def.SamplesZ - 1)
	if fx < 0 {
		fx = 0
	} else if fx > maxX {
		fx = maxX
	}
	if fz < 0 {
		fz = 0
	} else if fz > maxZ {
		fz = maxZ
	}
	x0 := int(math.Floor(float64(fx)))
	z0 := int(math.Floor(float64(fz)))
	x1 := x0 + 1
	z1 := z0 + 1
	if x1 >= def.SamplesX {
		x1 = x0
	}
	if z1 >= def.SamplesZ {
		z1 = z0
	}
	tx := fx - float32(x0)
	tz := fz - float32(z0)
	a := f.values[z0*def.SamplesX+x0]*(1-tx) + f.values[z0*def.SamplesX+x1]*tx
	b := f.values[z1*def.SamplesX+x0]*(1-tx) + f.values[z1*def.SamplesX+x1]*tx
	return a*(1-tz) + b*tz, true
}

func sampleCount(x, z int) (int, bool) {
	if x < 2 || z < 2 || x > math.MaxInt/z {
		return 0, false
	}
	return x * z, true
}

func safeRelativePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func finite(value float32) bool {
	f := float64(value)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func positiveFinite(value float32) bool {
	return finite(value) && value > 0
}

func near(a, b, tolerance float32) bool {
	return float32(math.Abs(float64(a-b))) <= tolerance
}
