package spatial

import (
	"math"
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

type readWindowKey struct {
	minX int32
	maxX int32
	minZ int32
	maxZ int32
}

// ReadFrame 是 replication pass 使用的 immutable spatial view。
// entities 與 cell membership 在 Reset 後直到下一次 Reset 都不再變動；
// candidateCache 只做同一個 frame 內的 query-window memoization，
// 不共享 Session-specific Layer / height / radius filtering semantics。
type ReadFrame struct {
	cellSize       float32
	entities       []world.EntityState
	cells          map[cellKey][]int
	candidateCache map[readWindowKey][]int
}

func (f *ReadFrame) Reset(cellSize float32, entities []world.EntityState) {
	if cellSize <= 0 {
		panic("spatial: read frame cellSize must be > 0")
	}
	f.cellSize = cellSize
	f.entities = entities
	if f.cells == nil {
		f.cells = make(map[cellKey][]int)
	} else {
		clear(f.cells)
	}
	if f.candidateCache == nil {
		f.candidateCache = make(map[readWindowKey][]int)
	} else {
		clear(f.candidateCache)
	}
	for i := range entities {
		key := f.cellFor(entities[i].Transform.Position)
		f.cells[key] = append(f.cells[key], i)
	}
}

// QueryRadiusInto 只重用 shared cell candidate view；精確 Layer / height / radius
// filtering 仍逐 Session 執行，讓未來 visibility policy 可以獨立擴充。
// dst 可由 caller 重用，避免每個 Session 配置新的 visible slice。
func (f *ReadFrame) QueryRadiusInto(center world.Position, radius float32, options QueryOptions, dst []int) ([]int, QueryStats) {
	dst = dst[:0]
	if radius < 0 || f.cellSize <= 0 {
		return dst, QueryStats{}
	}

	minX := f.cellCoord(center.X - radius)
	maxX := f.cellCoord(center.X + radius)
	minZ := f.cellCoord(center.Z - radius)
	maxZ := f.cellCoord(center.Z + radius)
	key := readWindowKey{minX: minX, maxX: maxX, minZ: minZ, maxZ: maxZ}
	stats := QueryStats{VisitedCells: int(maxX-minX+1) * int(maxZ-minZ+1)}

	candidates, reused := f.candidateCache[key]
	if reused {
		stats.SharedCandidateReuses = 1
	} else {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				candidates = append(candidates, f.cells[cellKey{X: x, Z: z}]...)
			}
		}
		// Entity frame 本身以 EntityID 排序，因此 index 排序同時恢復 stable EntityID order。
		sort.Ints(candidates)
		f.candidateCache[key] = candidates
		stats.SharedCandidateBuilds = 1
		stats.SharedCandidateScans = len(candidates)
	}

	stats.CandidateEntities = len(candidates)
	radiusSq := radius * radius
	for _, index := range candidates {
		e := f.entities[index]
		position := e.Transform.Position
		if options.SameLayer && position.Layer != center.Layer {
			continue
		}
		if options.MaxHeightDelta > 0 && abs32(position.Y-center.Y) > options.MaxHeightDelta {
			continue
		}
		if center.DistanceXZSquared(position) <= radiusSq {
			dst = append(dst, index)
		}
	}
	stats.MatchedEntities = len(dst)
	return dst, stats
}

func (f *ReadFrame) cellFor(position world.Position) cellKey {
	return cellKey{X: f.cellCoord(position.X), Z: f.cellCoord(position.Z)}
}

func (f *ReadFrame) cellCoord(value float32) int32 {
	return int32(math.Floor(float64(value / f.cellSize)))
}
