package replication

import (
	"sort"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

func (s *Service) BuildFrameLifecycleFirst(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrameLifecycleFirst(state, selfID, lastProcessedInput, frame, visibleIndices, false, limits)
}

func (s *Service) BuildFrameBorrowedLifecycleFirst(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrameLifecycleFirst(state, selfID, lastProcessedInput, frame, visibleIndices, true, limits)
}

func (s *Service) buildFrameLifecycleFirst(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, borrowSnapshotStorage bool, limits LifecycleLimits) Batch {
	desiredChanged := !sameDesiredIDs(state.desiredIDs, frame, visibleIndices)
	if desiredChanged {
		rebuildDesiredTracks(state, frame, visibleIndices)
		rebuildPendingDeparted(state)
	} else if len(state.departed) > 0 {
		prunePendingDeparted(state)
	}
	if limits.MaxMessages < 0 {
		return s.buildDeferredLifecycleFrame(state, selfID, lastProcessedInput, frame)
	}
	firstUnknown := firstUnknownDesired(state)
	if firstUnknown < 0 {
		return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, borrowSnapshotStorage, limits)
	}
	return s.buildBootstrapLifecycleFrame(state, selfID, lastProcessedInput, frame, visibleIndices, firstUnknown, limits)
}

func rebuildPendingDeparted(state *viewState) {
	state.departed = state.departed[:0]
	for id := range state.known {
		if !containsDesiredID(state.desiredIDs, id) {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
}

func prunePendingDeparted(state *viewState) {
	write := 0
	for _, id := range state.departed {
		if _, known := state.known[id]; !known || containsDesiredID(state.desiredIDs, id) {
			continue
		}
		state.departed[write] = id
		write++
	}
	state.departed = state.departed[:write]
}

func firstUnknownDesired(state *viewState) int {
	for i := range state.tracks {
		if !state.tracks[i].known {
			return i
		}
	}
	return -1
}

func (s *Service) buildDeferredLifecycleFrame(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame) Batch {
	state.buildNumber++
	state.messages = state.messages[:0]
	batch := Batch{Messages: state.messages}
	if firstUnknownDesired(state) >= 0 {
		batch.Stats.SpawnDeferred = 1
	}
	if len(state.departed) > 0 {
		batch.Stats.DespawnDeferred = len(state.departed)
	}
	if self, _, ok := frame.Entity(selfID); ok {
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryRealtimeSequenced,
			Message: protocol.PositionCorrection{
				Tick:                       frame.Tick,
				EntityID:                   self.ID,
				Position:                   self.Transform.Position,
				Yaw:                        self.Transform.Yaw,
				LastProcessedInputSequence: lastProcessedInput,
			},
		})
	}
	state.messages = batch.Messages
	return batch
}

func (s *Service) buildBootstrapLifecycleFrame(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, firstUnknown int, limits LifecycleLimits) Batch {
	state.buildNumber++
	state.messages = state.messages[:0]
	batch := Batch{Messages: state.messages}

	batch.Stats.DespawnCandidates = len(state.departed)
	for i, id := range state.departed {
		totalLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
		if !limits.allowMessage(totalLifecycle) || !limits.allowDespawn(batch.Stats.DespawnSelected) {
			batch.Stats.DespawnDeferred = len(state.departed) - i
			break
		}
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryReliableOrdered,
			Message:  protocol.EntityDespawn{EntityID: id},
		})
		batch.Stats.DespawnSelected++
	}

	for i := firstUnknown; i < len(visibleIndices) && i < len(state.tracks); i++ {
		index := visibleIndices[i]
		if index < 0 || index >= len(frame.Entities) {
			continue
		}
		track := &state.tracks[i]
		if track.known {
			continue
		}
		batch.Stats.SpawnCandidates++
		totalLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
		if !limits.allowMessage(totalLifecycle) || !limits.allowSpawn(batch.Stats.SpawnSelected) {
			batch.Stats.SpawnDeferred = 1
			break
		}
		e := frame.Entities[index]
		generation := frame.TransformGenerations[index]
		// Reliable Spawn 本身已攜帶同一份 authoritative transform。這裡先記在 unknown track；
		// TrySend 若失敗，known 仍為 false，realtime scheduler 不會誤用；retry 會覆寫最新 generation。
		// ConfirmSpawn 成功把 known=true 後，Spawn transform 即成為 dirty/refresh scheduler 的有效 baseline。
		track.lastDeliveredGeneration = generation
		track.lastSentBuild = state.buildNumber
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryReliableOrdered,
			Message: protocol.EntitySpawn{
				EntityID: e.ID,
				Kind:     e.Kind,
				Transform: protocol.EntityTransform{
					EntityID: e.ID,
					Tick:     frame.Tick,
					Position: e.Transform.Position,
					Yaw:      e.Transform.Yaw,
				},
			},
		})
		batch.Stats.SpawnSelected++
	}

	if self, _, ok := frame.Entity(selfID); ok {
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryRealtimeSequenced,
			Message: protocol.PositionCorrection{
				Tick:                       frame.Tick,
				EntityID:                   self.ID,
				Position:                   self.Transform.Position,
				Yaw:                        self.Transform.Yaw,
				LastProcessedInputSequence: lastProcessedInput,
			},
		})
	}

	state.messages = batch.Messages
	return batch
}
