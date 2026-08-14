package replication

import (
	"sort"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

// BuildFrameLifecycleFirst 在 view 尚有 unknown desired entity 時只做 bounded Reliable lifecycle
// 與 self correction；Spawn 自身已帶 authoritative transform，因此 bootstrap 尚未完成前不重複支付
// remote snapshot candidate scheduling。當所有 desired entity 都 known 後，自動回到完整 BuildFrame path。
func (s *Service) BuildFrameLifecycleFirst(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrameLifecycleFirst(state, selfID, lastProcessedInput, frame, visibleIndices, false, limits)
}

// BuildFrameBorrowedLifecycleFirst 是 ImmediateRealtimeConnection 的 lifecycle-first production path。
func (s *Service) BuildFrameBorrowedLifecycleFirst(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrameLifecycleFirst(state, selfID, lastProcessedInput, frame, visibleIndices, true, limits)
}

func (s *Service) buildFrameLifecycleFirst(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, borrowSnapshotStorage bool, limits LifecycleLimits) Batch {
	if !sameDesiredIDs(state.desiredIDs, frame, visibleIndices) {
		rebuildDesiredTracks(state, frame, visibleIndices)
	}
	if !hasUnknownDesired(state) {
		return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, borrowSnapshotStorage, limits)
	}
	return s.buildBootstrapLifecycleFrame(state, selfID, lastProcessedInput, frame, visibleIndices, limits)
}

func hasUnknownDesired(state *viewState) bool {
	for i := range state.tracks {
		if !state.tracks[i].known {
			return true
		}
	}
	return false
}

func (s *Service) buildBootstrapLifecycleFrame(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state.buildNumber++
	state.messages = state.messages[:0]
	batch := Batch{Messages: state.messages}

	for i, index := range visibleIndices {
		if i >= len(state.tracks) || index < 0 || index >= len(frame.Entities) {
			continue
		}
		track := &state.tracks[i]
		if track.known {
			continue
		}
		e := frame.Entities[index]
		batch.Stats.SpawnCandidates++
		if !limits.allowSpawn(batch.Stats.SpawnSelected) {
			batch.Stats.SpawnDeferred++
			continue
		}
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

	state.departed = state.departed[:0]
	for id := range state.known {
		if !containsDesiredID(state.desiredIDs, id) {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
	batch.Stats.DespawnCandidates = len(state.departed)
	for _, id := range state.departed {
		if !limits.allowDespawn(batch.Stats.DespawnSelected) {
			batch.Stats.DespawnDeferred++
			continue
		}
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryReliableOrdered,
			Message:  protocol.EntityDespawn{EntityID: id},
		})
		batch.Stats.DespawnSelected++
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
