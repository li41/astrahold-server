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
	firstUnknown := firstUnknownDesired(state)
	if firstUnknown < 0 {
		return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, borrowSnapshotStorage, limits)
	}
	return s.buildBootstrapLifecycleFrame(state, selfID, lastProcessedInput, frame, visibleIndices, firstUnknown, limits)
}

func firstUnknownDesired(state *viewState) int {
	for i := range state.tracks {
		if !state.tracks[i].known {
			return i
		}
	}
	return -1
}

func (s *Service) buildBootstrapLifecycleFrame(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, firstUnknown int, limits LifecycleLimits) Batch {
	state.buildNumber++
	state.messages = state.messages[:0]
	batch := Batch{Messages: state.messages}

	// AOI churn 同時產生 departed + unknown 時先清掉 stale known lifecycle。
	// 這可避免 combined/global budget 下舊 Entity 長時間留在 Client；mass join 沒有 departed，因此不改 S3-E.6 bootstrap throughput。
	state.departed = state.departed[:0]
	for id := range state.known {
		if !containsDesiredID(state.desiredIDs, id) {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
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

	// 從最早未完成的 desired track 開始；一旦 per-session 或 combined quantum 用完就立即停止。
	// Deferred work 不應為了完整統計而先掃過一次，下一個 build 會從新的 firstUnknown 繼續。
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
			// lifecycle-first path 的 Deferred 只表示「本 build 尚有更多工作」，
			// 不為了取得完整 deferred cardinality 而掃完整份 AOI。
			batch.Stats.SpawnDeferred = 1
			break
		}
		e := frame.Entities[index]
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
