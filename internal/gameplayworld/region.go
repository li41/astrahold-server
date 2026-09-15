package gameplayworld

import (
	"fmt"
	"math"

	"github.com/li41/astrahold-server/internal/world"
)

// RegionID 是跨系統可持久引用的穩定地理區域 ID；不承載 Client asset path 或 presentation identity。
type RegionID string

type PointXZ struct {
	X float32 `json:"x"`
	Z float32 `json:"z"`
}

// RegionCore 定義 Server-owned 地理 membership polygon。
// Region core 可彼此重疊（例如子區），不直接代表 collision 或 movement blocker。
type RegionCore struct {
	ID       RegionID      `json:"id"`
	ParentID RegionID      `json:"parent_id,omitempty"`
	Layer    world.LayerID `json:"layer"`
	Polygon  []PointXZ     `json:"polygon"`
}

func (r RegionCore) Contains(x, z float32) bool {
	if len(r.Polygon) < 3 || !finite(x) || !finite(z) {
		return false
	}

	inside := false
	j := len(r.Polygon) - 1
	for i := 0; i < len(r.Polygon); i++ {
		a := r.Polygon[j]
		b := r.Polygon[i]
		if pointOnSegment(x, z, a, b) {
			return true
		}
		if (a.Z > z) != (b.Z > z) {
			crossX := (b.X-a.X)*(z-a.Z)/(b.Z-a.Z) + a.X
			if x < crossX {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// RegionsAt 回傳包含指定 world-space 點的全部 Region core，保留 Definition 中的穩定順序。
// 子區可與父區同時回傳；呼叫端不得假設結果只能有一個。
func (d Definition) RegionsAt(layer world.LayerID, x, z float32) []RegionCore {
	regions := make([]RegionCore, 0, 2)
	for _, region := range d.Regions {
		if region.Layer == layer && region.Contains(x, z) {
			regions = append(regions, region)
		}
	}
	return regions
}

func validateRegions(d Definition) error {
	regions := make(map[RegionID]RegionCore, len(d.Regions))
	for i, region := range d.Regions {
		if region.ID == "" || len(region.Polygon) < 3 {
			return fmt.Errorf("region[%d] id/polygon", i)
		}
		if _, exists := regions[region.ID]; exists {
			return fmt.Errorf("duplicate region id %q", region.ID)
		}

		var area2 float64
		for pointIndex, point := range region.Polygon {
			if !finite(point.X) || !finite(point.Z) {
				return fmt.Errorf("region %q point[%d] is not finite", region.ID, pointIndex)
			}
			if !pointCoveredBySurface(d.Surfaces, region.Layer, point.X, point.Z) {
				return fmt.Errorf("region %q point[%d] is outside layer %d surfaces", region.ID, pointIndex, region.Layer)
			}
			next := region.Polygon[(pointIndex+1)%len(region.Polygon)]
			area2 += float64(point.X)*float64(next.Z) - float64(next.X)*float64(point.Z)
		}
		if math.Abs(area2) < 1e-3 {
			return fmt.Errorf("region %q polygon has zero area", region.ID)
		}
		regions[region.ID] = region
	}

	for _, region := range d.Regions {
		if region.ParentID == "" {
			continue
		}
		parent, ok := regions[region.ParentID]
		if !ok {
			return fmt.Errorf("region %q parent missing: %q", region.ID, region.ParentID)
		}
		if parent.Layer != region.Layer {
			return fmt.Errorf("region %q parent layer mismatch: %q", region.ID, region.ParentID)
		}

		seen := map[RegionID]struct{}{region.ID: {}}
		current := region.ParentID
		for current != "" {
			if _, exists := seen[current]; exists {
				return fmt.Errorf("region %q parent cycle at %q", region.ID, current)
			}
			seen[current] = struct{}{}
			ancestor := regions[current]
			current = ancestor.ParentID
		}
	}
	return nil
}

func pointCoveredBySurface(surfaces []Surface, layer world.LayerID, x, z float32) bool {
	for _, surface := range surfaces {
		if surface.Layer == layer && surface.Bounds.Contains(x, z) {
			return true
		}
	}
	return false
}

func pointOnSegment(x, z float32, a, b PointXZ) bool {
	const epsilon = 1e-4
	cross := (x-a.X)*(b.Z-a.Z) - (z-a.Z)*(b.X-a.X)
	if float32(math.Abs(float64(cross))) > epsilon {
		return false
	}
	dot := (x-a.X)*(x-b.X) + (z-a.Z)*(z-b.Z)
	return dot <= epsilon
}
