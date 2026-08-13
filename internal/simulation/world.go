// Package simulation 組合世界實體、AOI 與移動服務。
package simulation

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrEntityExists   = errors.New("simulation: entity already exists")
	ErrEntityNotFound = errors.New("simulation: entity not found")
)

type actor struct {
	entity world.EntityState
	move   movement.AgentState
}

// TickError 表示單一實體在某個 server tick 中的移動失敗。
type TickError struct {
	EntityID world.EntityID
	Err      error
}

// World 是單一 zone/world simulation 的第一版容器。
//
// 目前假設由單一 simulation goroutine 呼叫，因此不放 mutex。
// 未來網路層透過 command queue 將輸入送進世界執行緒。
type World struct {
	actors   map[world.EntityID]*actor
	spatial  *spatial.Grid
	movement *movement.Service
}

func New(spatialIndex *spatial.Grid, movementService *movement.Service) *World {
	if spatialIndex == nil || movementService == nil {
		panic("simulation: spatial index and movement service are required")
	}
	return &World{
		actors:   make(map[world.EntityID]*actor),
		spatial:  spatialIndex,
		movement: movementService,
	}
}

// Spawn 加入一個可移動實體。
func (w *World) Spawn(entity world.EntityState, speed, radius, maxStepHeight float32) error {
	if _, exists := w.actors[entity.ID]; exists {
		return ErrEntityExists
	}

	a := &actor{
		entity: entity,
		move: movement.AgentState{
			Position:      entity.Transform.Position,
			Speed:         speed,
			Radius:        radius,
			MaxStepHeight: maxStepHeight,
		},
	}
	w.actors[entity.ID] = a
	w.spatial.Upsert(entity.ID, entity.Transform.Position)
	return nil
}

func (w *World) Remove(id world.EntityID) {
	delete(w.actors, id)
	w.spatial.Remove(id)
}

// SetMoveInput 只更新 client 的移動意圖；實際位置要等下一個 server Tick 才改變。
func (w *World) SetMoveInput(id world.EntityID, input movement.Input) error {
	a, ok := w.actors[id]
	if !ok {
		return ErrEntityNotFound
	}
	return w.movement.AcceptInput(&a.move, input)
}

// Tick 由 server clock 推進世界。
//
// 某個 actor 被阻擋不會中止整個 world tick；錯誤會逐一回報，方便之後產生 correction/event。
func (w *World) Tick(deltaSeconds float32) []TickError {
	ids := make([]world.EntityID, 0, len(w.actors))
	for id := range w.actors {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var tickErrors []TickError
	for _, id := range ids {
		a := w.actors[id]
		position, err := w.movement.Step(&a.move, deltaSeconds)
		if err != nil {
			tickErrors = append(tickErrors, TickError{EntityID: id, Err: err})
			continue
		}
		a.entity.Transform.Position = position
		w.spatial.Upsert(id, position)
	}
	return tickErrors
}

func (w *World) Entity(id world.EntityID) (world.EntityState, bool) {
	a, ok := w.actors[id]
	if !ok {
		return world.EntityState{}, false
	}
	return a.entity, true
}

// QueryAOI 回傳中心附近的實體 snapshot。
func (w *World) QueryAOI(center world.Position, radius float32, options spatial.QueryOptions) []world.EntityState {
	ids := w.spatial.QueryRadius(center, radius, options)
	result := make([]world.EntityState, 0, len(ids))
	for _, id := range ids {
		if a, ok := w.actors[id]; ok {
			result = append(result, a.entity)
		}
	}
	return result
}

// Snapshot 回傳穩定排序的全世界 entity snapshot，主要供測試與管理工具使用。
func (w *World) Snapshot() []world.EntityState {
	result := make([]world.EntityState, 0, len(w.actors))
	for _, a := range w.actors {
		result = append(result, a.entity)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
