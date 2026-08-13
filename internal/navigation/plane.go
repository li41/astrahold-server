package navigation

import "github.com/li41/astrahold-server/internal/world"

// Plane 是第一階段用來驗證新世界核心的簡單導航面。
// 它不是正式地圖格式，之後會由 World Compiler / NavMesh 實作取代。
type Plane struct {
	MinX   float32
	MaxX   float32
	MinZ   float32
	MaxZ   float32
	Height float32
	Layer  world.LayerID
}

func (p Plane) ResolveMove(from world.Position, displacement world.Vec3, _ Agent) (world.Position, error) {
	if from.Layer != p.Layer {
		return from, ErrUnsupportedLayer
	}

	next := from.Add(world.Vec3{X: displacement.X, Z: displacement.Z})
	if next.X < p.MinX || next.X > p.MaxX || next.Z < p.MinZ || next.Z > p.MaxZ {
		return from, ErrBlocked
	}
	next.Y = p.Height
	next.Layer = p.Layer
	return next, nil
}

func (p Plane) HasLineOfSight(from, to world.Position) bool {
	if from.Layer != p.Layer || to.Layer != p.Layer {
		return false
	}
	return from.X >= p.MinX && from.X <= p.MaxX && from.Z >= p.MinZ && from.Z <= p.MaxZ &&
		to.X >= p.MinX && to.X <= p.MaxX && to.Z >= p.MinZ && to.Z <= p.MaxZ
}
