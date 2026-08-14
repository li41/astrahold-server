// Package replication 將 AOI 世界狀態轉成 client 可消費的 spawn/despawn/snapshot/correction 訊息。
package replication

import (
	"sort"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type Outbound struct {
	Delivery protocol.Delivery
	Message  protocol.Message
}

type Batch struct{ Messages []Outbound }

type viewState struct {
	known    map[world.EntityID]struct{}
	scratch  map[world.EntityID]struct{}
	departed []world.EntityID
}

type Service struct {
	views map[session.ID]*viewState
}

func NewService() *Service { return &Service{views: make(map[session.ID]*viewState)} }

func (s *Service) Register(id session.ID) {
	if _, ok := s.views[id]; ok {
		return
	}
	s.views[id] = &viewState{known: make(map[world.EntityID]struct{}), scratch: make(map[world.EntityID]struct{})}
}

func (s *Service) Remove(id session.ID) { delete(s.views, id) }

// Knows 回報該 Session 是否已經收過 EntitySpawn，供低頻 Reliable entity state 做 AOI fan-out。
func (s *Service) Knows(sessionID session.ID, entityID world.EntityID) bool {
	state := s.views[sessionID]
	if state == nil {
		return false
	}
	_, ok := state.known[entityID]
	return ok
}

func (s *Service) Build(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, tick uint64, visible []world.EntityState) Batch {
	state := s.views[sessionID]
	if state == nil {
		state = &viewState{known: make(map[world.EntityID]struct{}), scratch: make(map[world.EntityID]struct{})}
		s.views[sessionID] = state
	}

	ordered := visible
	if !sort.SliceIsSorted(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID }) {
		ordered = append([]world.EntityState(nil), visible...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	}

	clear(state.scratch)
	batch := Batch{Messages: make([]Outbound, 0, len(ordered)+4)}
	transforms := make([]protocol.EntityTransform, len(ordered))
	var selfTransform protocol.EntityTransform
	hasSelf := false

	for i := range ordered {
		e := ordered[i]
		state.scratch[e.ID] = struct{}{}
		tr := protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		transforms[i] = tr
		if _, ok := state.known[e.ID]; !ok {
			batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntitySpawn{EntityID: e.ID, Kind: e.Kind, Transform: tr}})
		}
		if e.ID == selfID {
			selfTransform = tr
			hasSelf = true
		}
	}

	state.departed = state.departed[:0]
	for id := range state.known {
		if _, ok := state.scratch[id]; !ok {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
	for _, id := range state.departed {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntityDespawn{EntityID: id}})
	}

	chunkCount := (len(transforms) + protocol.MaxSnapshotEntitiesPerChunk - 1) / protocol.MaxSnapshotEntitiesPerChunk
	if chunkCount == 0 {
		chunkCount = 1
	}
	for chunk := 0; chunk < chunkCount; chunk++ {
		start := chunk * protocol.MaxSnapshotEntitiesPerChunk
		end := start + protocol.MaxSnapshotEntitiesPerChunk
		if end > len(transforms) {
			end = len(transforms)
		}
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.WorldSnapshot{Tick: tick, ChunkIndex: uint16(chunk), ChunkCount: uint16(chunkCount), Entities: transforms[start:end]}})
	}

	if hasSelf {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.PositionCorrection{Tick: tick, EntityID: selfTransform.EntityID, Position: selfTransform.Position, Yaw: selfTransform.Yaw, LastProcessedInputSequence: lastProcessedInput}})
	}

	state.known, state.scratch = state.scratch, state.known
	return batch
}
