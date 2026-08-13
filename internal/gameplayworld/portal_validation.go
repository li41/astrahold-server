package gameplayworld

import (
	"fmt"
	"math"

	"github.com/li41/astrahold-server/internal/world"
)

// ValidatePortalGeometry 驗證 Portal trigger 同時位於 source/target Surface，且整個 trigger
// 範圍內兩個 Surface 的高度差不超過 agent MaxStepHeight。
// 這讓 Layer transition 是 world topology contract，而不是由 simulation tick 步長偶然決定。
func ValidatePortalGeometry(d Definition) error {
	for _, portal := range d.Portals {
		from, ok := surfaceContainingBounds(d.Surfaces, portal.FromLayer, portal.Bounds)
		if !ok {
			return fmt.Errorf("portal %q trigger 不完整落在 from_layer=%d surface", portal.ID, portal.FromLayer)
		}
		to, ok := surfaceContainingBounds(d.Surfaces, portal.ToLayer, portal.Bounds)
		if !ok {
			return fmt.Errorf("portal %q trigger 不完整落在 to_layer=%d surface", portal.ID, portal.ToLayer)
		}

		corners := [][2]float32{
			{portal.Bounds.MinX, portal.Bounds.MinZ},
			{portal.Bounds.MaxX, portal.Bounds.MinZ},
			{portal.Bounds.MaxX, portal.Bounds.MaxZ},
			{portal.Bounds.MinX, portal.Bounds.MaxZ},
		}
		for _, corner := range corners {
			x, z := corner[0], corner[1]
			delta := float32(math.Abs(float64(from.Plane.HeightAt(x, z) - to.Plane.HeightAt(x, z))))
			if delta > d.Agent.MaxStepHeight+0.0001 {
				return fmt.Errorf(
					"portal %q surface 高度差 %.3fm 超過 max_step_height %.3fm at (%.3f, %.3f)",
					portal.ID, delta, d.Agent.MaxStepHeight, x, z,
				)
			}
		}
	}
	return nil
}

func surfaceContainingBounds(surfaces []Surface, layer world.LayerID, bounds BoundsXZ) (Surface, bool) {
	for _, surface := range surfaces {
		if surface.Layer == layer && containsBounds(surface.Bounds, bounds) {
			return surface, true
		}
	}
	return Surface{}, false
}

func containsBounds(outer, inner BoundsXZ) bool {
	return inner.MinX >= outer.MinX && inner.MaxX <= outer.MaxX &&
		inner.MinZ >= outer.MinZ && inner.MaxZ <= outer.MaxZ
}
