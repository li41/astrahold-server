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
		// Mass join 的 common path 是 EntityID stable order 尾端持續追加。這種 change
		// 不可能產生 Despawn，也不需要把既有 dense tracks 全部經 known/maps 重建一次。
		if desiredAppendOnly(state.desiredIDs, frame, visibleIndices) {
			appendDesiredTracks(state, frame, visibleIndices)
			if len(state.departed) > 0 {
				prunePendingDeparted(state)
			}
		} else {
			// 一般 membership change 才做 subset/removal 判斷與完整 dense rebuild。
			membershipRemoved := desiredMembershipRemoved(state.desiredIDs, frame, visibleIndices)
			rebuildDesiredTracks(state, frame, visibleIndices)
			if membershipRemoved {
				rebuildPendingDeparted(state)
			} else if len(state.departed) > 0 {
				prunePendingDeparted(state)
			}
		}
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

// desiredAppendOnly 判斷新 desired 是否只是 stable EntityID order 的尾端追加。
// 只有完全保留舊 prefix 且新長度更長才走 fast path；中間插入或任何 removal 都回一般 rebuild。
func desiredAppendOnly(previous []world.EntityID, frame *simulation.ReplicationFrame, visibleIndices []int) bool {
	if len(previous) == 0 || len(visibleIndices) <= len(previous) {
		return false
	}
	for i, id := range previous {
		index := visibleIndices[i]
		if index < 0 || index >= len(frame.Entities) || frame.Entities[index].ID != id {
			return false
		}
	}
	return true
}

// appendDesiredTracks 保留既有 dense track state，只初始化新增尾段。
// 這避免 500-client ramp-up 每次新 peer 加入時，對每個 Session 重做整份 known/map lookup。
func appendDesiredTracks(state *viewState, frame *simulation.ReplicationFrame, visibleIndices []int) {
	oldCount := len(state.desiredIDs)
	count := len(visibleIndices)

	if cap(state.desiredIDs) < count {
		capacity := count
		if doubled := cap(state.desiredIDs) * 2; doubled > capacity {
			capacity = doubled
		}
		ids := make([]world.EntityID, count, capacity)
		copy(ids, state.desiredIDs)
		state.desiredIDs = ids
	} else {
		state.desiredIDs = state.desiredIDs[:count]
	}

	if cap(state.tracks) < count {
		capacity := count
		if doubled := cap(state.tracks) * 2; doubled > capacity {
			capacity = doubled
		}
		tracks := make([]entityTrack, count, capacity)
		copy(tracks, state.tracks)
		state.tracks = tracks
	} else {
		state.tracks = state.tracks[:count]
		clear(state.tracks[oldCount:])
	}

	for i := oldCount; i < count; i++ {
		index := visibleIndices[i]
		if index < 0 || index >= len(frame.Entities) {
			continue
		}
		id := frame.Entities[index].ID
		_, known := state.known[id]
		state.desiredIDs[i] = id
		state.tracks[i] = entityTrack{
			id:                      id,
			known:                   known,
			lastDeliveredGeneration: state.lastDeliveredGeneration[id],
			lastSentBuild:           state.lastSentBuild[id],
		}
	}
}

// desiredMembershipRemoved 利用 desired / frame 的 stable EntityID order 做 two-pointer subset 檢查。
// true 代表至少一個舊 desired ID 不在新 AOI；只有這種 membership change 才需要掃 known 建 Despawn list。
func desiredMembershipRemoved(previous []world.EntityID, frame *simulation.ReplicationFrame, visibleIndices []int) bool {
	if len(previous) == 0 {
		return false
	}
	previousIndex := 0
	for _, frameIndex := range visibleIndices {
		if frameIndex < 0 || frameIndex >= len(frame.Entities) {
			continue
		}
		id := frame.Entities[frameIndex].ID
		for previousIndex < len(previous) && previous[previousIndex] < id {
			return true
		}
		if previousIndex < len(previous) && previous[previousIndex] == id {
			previousIndex++
			if previousIndex == len(previous) {
				return false
			}
		}
	}
	return previousIndex < len(previous)
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

		// Reliable Spawn 本身已攜帶這一代 authoritative transform。unknown track 在 TrySend
		// 成功前仍不會進 realtime scheduler，但先同步 rare membership-rebuild mirror，避免
		// mass join desired 持續成長時重建 tracks 後把已 materialize/即將確認的 transform
		// 全部降回 generation=0，進而在 bootstrap 尾端製造十萬級 dirty candidates。
		// 若 TrySend 失敗，known 仍為 false；下一次 retry 會用最新 generation 覆寫 mirror，
		// 因此 lifecycle truth 仍只由 ConfirmSpawn 成功決定。
		track.lastDeliveredGeneration = generation
		track.lastSentBuild = state.buildNumber
		state.lastDeliveredGeneration[e.ID] = generation
		state.lastSentBuild[e.ID] = state.buildNumber

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
