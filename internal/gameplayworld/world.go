// Package gameplayworld 定義 World Compiler 與 Go Server 之間的版本化 Gameplay Proxy schema。
package gameplayworld

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/li41/astrahold-server/internal/world"
)

const SchemaVersion uint16 = 1

var (
	ErrUnsupportedSchema = errors.New("gameplayworld: unsupported schema version")
	ErrInvalidDefinition = errors.New("gameplayworld: invalid definition")
)

type AgentDefaults struct {
	Radius        float32 `json:"radius"`
	Height        float32 `json:"height"`
	MaxStepHeight float32 `json:"max_step_height"`
}

type BoundsXZ struct {
	MinX float32 `json:"min_x"`
	MaxX float32 `json:"max_x"`
	MinZ float32 `json:"min_z"`
	MaxZ float32 `json:"max_z"`
}

func (b BoundsXZ) Contains(x, z float32) bool {
	return x >= b.MinX && x <= b.MaxX && z >= b.MinZ && z <= b.MaxZ
}

func (b BoundsXZ) Expanded(amount float32) BoundsXZ {
	return BoundsXZ{MinX: b.MinX - amount, MaxX: b.MaxX + amount, MinZ: b.MinZ - amount, MaxZ: b.MaxZ + amount}
}

type SurfacePlane struct {
	OriginX float32 `json:"origin_x"`
	OriginZ float32 `json:"origin_z"`
	BaseY   float32 `json:"base_y"`
	SlopeX  float32 `json:"slope_x"`
	SlopeZ  float32 `json:"slope_z"`
}

func (p SurfacePlane) HeightAt(x, z float32) float32 {
	return p.BaseY + (x-p.OriginX)*p.SlopeX + (z-p.OriginZ)*p.SlopeZ
}

type Surface struct {
	ID     string        `json:"id"`
	Layer  world.LayerID `json:"layer"`
	Bounds BoundsXZ      `json:"bounds"`
	Plane  SurfacePlane  `json:"plane"`
}

type Portal struct {
	ID            string        `json:"id"`
	FromLayer     world.LayerID `json:"from_layer"`
	ToLayer       world.LayerID `json:"to_layer"`
	Bounds        BoundsXZ      `json:"bounds"`
	Bidirectional bool          `json:"bidirectional"`
}

type Blocker struct {
	ID             string        `json:"id"`
	Layer          world.LayerID `json:"layer"`
	Bounds         BoundsXZ      `json:"bounds"`
	MinY           float32       `json:"min_y"`
	MaxY           float32       `json:"max_y"`
	BlocksMovement bool          `json:"blocks_movement"`
	BlocksLOS      bool          `json:"blocks_los"`
	Enabled        bool          `json:"enabled"`
}

type Definition struct {
	SchemaVersion uint16        `json:"schema_version"`
	WorldID       string        `json:"world_id"`
	Revision      string        `json:"revision"`
	Units         string        `json:"units"`
	Agent         AgentDefaults `json:"agent"`
	Surfaces      []Surface     `json:"surfaces"`
	Portals       []Portal      `json:"portals"`
	Blockers      []Blocker     `json:"blockers"`
}

type Loaded struct {
	Definition Definition
	SHA256     string
}

func LoadFile(path string) (Loaded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	return Load(bytes.NewReader(data))
}

func Load(r io.Reader) (Loaded, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Loaded{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition Definition
	if err := decoder.Decode(&definition); err != nil {
		return Loaded{}, fmt.Errorf("%w: decode: %v", ErrInvalidDefinition, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Loaded{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidDefinition)
		}
		return Loaded{}, fmt.Errorf("%w: trailing data: %v", ErrInvalidDefinition, err)
	}
	if err := Validate(definition); err != nil {
		return Loaded{}, err
	}
	digest := sha256.Sum256(data)
	return Loaded{Definition: definition, SHA256: hex.EncodeToString(digest[:])}, nil
}

func Validate(d Definition) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, d.SchemaVersion, SchemaVersion)
	}
	if d.WorldID == "" || d.Revision == "" || d.Units != "meters" {
		return fmt.Errorf("%w: world_id/revision/units", ErrInvalidDefinition)
	}
	if !positiveFinite(d.Agent.Radius) || !positiveFinite(d.Agent.Height) || !positiveFinite(d.Agent.MaxStepHeight) {
		return fmt.Errorf("%w: agent defaults", ErrInvalidDefinition)
	}
	if len(d.Surfaces) == 0 {
		return fmt.Errorf("%w: at least one surface is required", ErrInvalidDefinition)
	}

	surfaceIDs := make(map[string]struct{}, len(d.Surfaces))
	layers := make(map[world.LayerID]struct{})
	for i, surface := range d.Surfaces {
		if surface.ID == "" || !validBounds(surface.Bounds) || !finitePlane(surface.Plane) {
			return fmt.Errorf("%w: surface[%d]", ErrInvalidDefinition, i)
		}
		if _, exists := surfaceIDs[surface.ID]; exists {
			return fmt.Errorf("%w: duplicate surface id %q", ErrInvalidDefinition, surface.ID)
		}
		surfaceIDs[surface.ID] = struct{}{}
		layers[surface.Layer] = struct{}{}
	}
	for i := 0; i < len(d.Surfaces); i++ {
		for j := i + 1; j < len(d.Surfaces); j++ {
			a, b := d.Surfaces[i], d.Surfaces[j]
			if a.Layer == b.Layer && overlapsInterior(a.Bounds, b.Bounds) {
				return fmt.Errorf("%w: overlapping surfaces on layer %d: %s/%s", ErrInvalidDefinition, a.Layer, a.ID, b.ID)
			}
		}
	}

	portalIDs := make(map[string]struct{}, len(d.Portals))
	for i, portal := range d.Portals {
		if portal.ID == "" || !validBounds(portal.Bounds) || portal.FromLayer == portal.ToLayer {
			return fmt.Errorf("%w: portal[%d]", ErrInvalidDefinition, i)
		}
		if _, ok := layers[portal.FromLayer]; !ok {
			return fmt.Errorf("%w: portal %q from_layer missing", ErrInvalidDefinition, portal.ID)
		}
		if _, ok := layers[portal.ToLayer]; !ok {
			return fmt.Errorf("%w: portal %q to_layer missing", ErrInvalidDefinition, portal.ID)
		}
		if _, exists := portalIDs[portal.ID]; exists {
			return fmt.Errorf("%w: duplicate portal id %q", ErrInvalidDefinition, portal.ID)
		}
		portalIDs[portal.ID] = struct{}{}
	}
	if err := ValidatePortalGeometry(d); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}

	blockerIDs := make(map[string]struct{}, len(d.Blockers))
	for i, blocker := range d.Blockers {
		if blocker.ID == "" || !validBounds(blocker.Bounds) || !finite(blocker.MinY) || !finite(blocker.MaxY) || blocker.MinY > blocker.MaxY {
			return fmt.Errorf("%w: blocker[%d]", ErrInvalidDefinition, i)
		}
		if !blocker.BlocksMovement && !blocker.BlocksLOS {
			return fmt.Errorf("%w: blocker %q blocks nothing", ErrInvalidDefinition, blocker.ID)
		}
		if blocker.BlocksMovement {
			if _, ok := layers[blocker.Layer]; !ok {
				return fmt.Errorf("%w: blocker %q layer missing", ErrInvalidDefinition, blocker.ID)
			}
		}
		if _, exists := blockerIDs[blocker.ID]; exists {
			return fmt.Errorf("%w: duplicate blocker id %q", ErrInvalidDefinition, blocker.ID)
		}
		blockerIDs[blocker.ID] = struct{}{}
	}
	return nil
}

func validBounds(b BoundsXZ) bool {
	return finite(b.MinX) && finite(b.MaxX) && finite(b.MinZ) && finite(b.MaxZ) && b.MinX < b.MaxX && b.MinZ < b.MaxZ
}

func finitePlane(p SurfacePlane) bool {
	return finite(p.OriginX) && finite(p.OriginZ) && finite(p.BaseY) && finite(p.SlopeX) && finite(p.SlopeZ)
}

func finite(v float32) bool {
	f := float64(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func positiveFinite(v float32) bool { return finite(v) && v > 0 }

func overlapsInterior(a, b BoundsXZ) bool {
	return a.MinX < b.MaxX && a.MaxX > b.MinX && a.MinZ < b.MaxZ && a.MaxZ > b.MinZ
}
