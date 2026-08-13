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
type Service struct {
	known map[session.ID]map[world.EntityID]struct{}
}

func NewService() *Service { return &Service{known: make(map[session.ID]map[world.EntityID]struct{})} }
func (s *Service) Register(id session.ID) {
	if _, ok := s.known[id]; !ok {
		s.known[id] = make(map[world.EntityID]struct{})
	}
}
func (s *Service) Remove(id session.ID) { delete(s.known, id) }

func (s *Service) Build(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, tick uint64, visible []world.EntityState) Batch {
	previous := s.known[sessionID]
	if previous == nil {
		previous = make(map[world.EntityID]struct{})
	}
	ordered := append([]world.EntityState(nil), visible...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	current := make(map[world.EntityID]struct{}, len(ordered))
	batch := Batch{}
	transforms := make([]protocol.EntityTransform, 0, len(ordered))
	var self *world.EntityState
	for i := range ordered {
		e := ordered[i]
		current[e.ID] = struct{}{}
		tr := protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		transforms = append(transforms, tr)
		if _, ok := previous[e.ID]; !ok {
			batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntitySpawn{EntityID: e.ID, Kind: e.Kind, Transform: tr}})
		}
		if e.ID == selfID {
			copy := e
			self = &copy
		}
	}
	departed := make([]world.EntityID, 0)
	for id := range previous {
		if _, ok := current[id]; !ok {
			departed = append(departed, id)
		}
	}
	sort.Slice(departed, func(i, j int) bool { return departed[i] < departed[j] })
	for _, id := range departed {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntityDespawn{EntityID: id}})
	}
	batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.WorldSnapshot{Tick: tick, Entities: transforms}})
	if self != nil {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.PositionCorrection{Tick: tick, EntityID: self.ID, Position: self.Transform.Position, Yaw: self.Transform.Yaw, LastProcessedInputSequence: lastProcessedInput}})
	}
	s.known[sessionID] = current
	return batch
}
