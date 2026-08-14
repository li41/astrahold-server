package simulation

import (
	"sort"

	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

// ReplicationFrame 是單一 snapshot pass 的 immutable read-side world view。
// Entities 以 EntityID stable order 排列；TransformGenerations 與 Entities 使用相同 index。
type ReplicationFrame struct {
	Tick                 uint64
	Entities             []world.EntityState
	TransformGenerations []uint64
	IndexByID            map[world.EntityID]int
	Spatial              spatial.ReadFrame
}

func (f *ReplicationFrame) Entity(id world.EntityID) (world.EntityState, uint64, bool) {
	index, ok := f.IndexByID[id]
	if !ok {
		return world.EntityState{}, 0, false
	}
	return f.Entities[index], f.TransformGenerations[index], true
}

// ReplicationFrameBuilder 由 world owner 持有並在 snapshot pass 開頭呼叫一次。
// 它只對每個 Entity 做一次 transform equality compare，將結果轉成 global generation；
// Session replication 之後只比較 generation，不再重複比較完整 Position / Yaw。
type ReplicationFrameBuilder struct {
	ids             []world.EntityID
	frame           ReplicationFrame
	lastTransforms  map[world.EntityID]world.Transform
	generations     map[world.EntityID]uint64
	nextGeneration  uint64
}

func NewReplicationFrameBuilder() *ReplicationFrameBuilder {
	return &ReplicationFrameBuilder{
		lastTransforms: make(map[world.EntityID]world.Transform),
		generations:    make(map[world.EntityID]uint64),
		frame: ReplicationFrame{IndexByID: make(map[world.EntityID]int)},
	}
}

func (b *ReplicationFrameBuilder) Build(w *World, tick uint64) *ReplicationFrame {
	b.ids = b.ids[:0]
	for id := range w.actors {
		b.ids = append(b.ids, id)
	}
	sort.Slice(b.ids, func(i, j int) bool { return b.ids[i] < b.ids[j] })

	if cap(b.frame.Entities) < len(b.ids) {
		b.frame.Entities = make([]world.EntityState, len(b.ids))
	} else {
		b.frame.Entities = b.frame.Entities[:len(b.ids)]
	}
	if cap(b.frame.TransformGenerations) < len(b.ids) {
		b.frame.TransformGenerations = make([]uint64, len(b.ids))
	} else {
		b.frame.TransformGenerations = b.frame.TransformGenerations[:len(b.ids)]
	}
	clear(b.frame.IndexByID)

	for i, id := range b.ids {
		entity := w.actors[id].entity
		previous, exists := b.lastTransforms[id]
		generation := b.generations[id]
		if !exists || previous != entity.Transform {
			b.nextGeneration++
			generation = b.nextGeneration
			b.generations[id] = generation
			b.lastTransforms[id] = entity.Transform
		}
		b.frame.Entities[i] = entity
		b.frame.TransformGenerations[i] = generation
		b.frame.IndexByID[id] = i
	}

	// 清除已離開世界的 generation state，避免長期 world churn 累積。
	for id := range b.lastTransforms {
		if _, ok := b.frame.IndexByID[id]; ok {
			continue
		}
		delete(b.lastTransforms, id)
		delete(b.generations, id)
	}

	b.frame.Tick = tick
	b.frame.Spatial.Reset(w.spatial.CellSize(), b.frame.Entities)
	return &b.frame
}
