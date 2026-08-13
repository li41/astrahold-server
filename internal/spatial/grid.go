// Package spatial 提供 MMO 用的空間索引。
//
// Astrahold 不再把 Grid Cell 當作世界座標；Grid 只負責快速找出附近實體。
package spatial

import (
	"math"
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

type cellKey struct {
	X int32
	Z int32
}

type entry struct {
	position world.Position
	cell     cellKey
}

// QueryOptions 控制 AOI 查詢的額外過濾條件。
type QueryOptions struct {
	// SameLayer 只回傳與查詢中心相同 Layer 的實體。
	SameLayer bool
	// MaxHeightDelta > 0 時，會額外限制 Y 軸高度差。
	MaxHeightDelta float32
}

// Grid 是以 X/Z 切格的空間索引。
//
// Grid 本身不是 thread-safe；目前設計由單一 world simulation goroutine 擁有並修改。
type Grid struct {
	cellSize float32
	cells    map[cellKey]map[world.EntityID]struct{}
	entries  map[world.EntityID]entry
}

// NewGrid 建立空間索引。cellSize 的單位為公尺。
func NewGrid(cellSize float32) *Grid {
	if cellSize <= 0 {
		panic("spatial: cellSize must be > 0")
	}
	return &Grid{
		cellSize: cellSize,
		cells:    make(map[cellKey]map[world.EntityID]struct{}),
		entries:  make(map[world.EntityID]entry),
	}
}

// Upsert 新增或更新實體位置。
func (g *Grid) Upsert(id world.EntityID, position world.Position) {
	newCell := g.cellFor(position)
	if old, ok := g.entries[id]; ok {
		if old.cell == newCell {
			g.entries[id] = entry{position: position, cell: newCell}
			return
		}
		g.removeFromCell(id, old.cell)
	}

	bucket := g.cells[newCell]
	if bucket == nil {
		bucket = make(map[world.EntityID]struct{})
		g.cells[newCell] = bucket
	}
	bucket[id] = struct{}{}
	g.entries[id] = entry{position: position, cell: newCell}
}

// Remove 將實體移出空間索引。
func (g *Grid) Remove(id world.EntityID) {
	old, ok := g.entries[id]
	if !ok {
		return
	}
	g.removeFromCell(id, old.cell)
	delete(g.entries, id)
}

// Position 取得目前索引中的權威位置。
func (g *Grid) Position(id world.EntityID) (world.Position, bool) {
	e, ok := g.entries[id]
	return e.position, ok
}

// QueryRadius 以 X/Z 水平距離查詢附近實體，並可用 Layer/高度差進一步過濾。
func (g *Grid) QueryRadius(center world.Position, radius float32, options QueryOptions) []world.EntityID {
	if radius < 0 {
		return nil
	}

	minX := g.cellCoord(center.X - radius)
	maxX := g.cellCoord(center.X + radius)
	minZ := g.cellCoord(center.Z - radius)
	maxZ := g.cellCoord(center.Z + radius)
	radiusSq := radius * radius

	result := make([]world.EntityID, 0, 16)
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			for id := range g.cells[cellKey{X: x, Z: z}] {
				e := g.entries[id]
				if options.SameLayer && e.position.Layer != center.Layer {
					continue
				}
				if options.MaxHeightDelta > 0 && abs32(e.position.Y-center.Y) > options.MaxHeightDelta {
					continue
				}
				if center.DistanceXZSquared(e.position) <= radiusSq {
					result = append(result, id)
				}
			}
		}
	}

	// 穩定順序方便 snapshot、測試與除錯。
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (g *Grid) cellFor(position world.Position) cellKey {
	return cellKey{X: g.cellCoord(position.X), Z: g.cellCoord(position.Z)}
}

func (g *Grid) cellCoord(value float32) int32 {
	return int32(math.Floor(float64(value / g.cellSize)))
}

func (g *Grid) removeFromCell(id world.EntityID, key cellKey) {
	bucket := g.cells[key]
	delete(bucket, id)
	if len(bucket) == 0 {
		delete(g.cells, key)
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
