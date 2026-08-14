package replication

import (
	"sync"

	"github.com/li41/astrahold-server/internal/simulation"
)

var lifecycleTrackScratchPool sync.Pool

// rebuildConvergedLifecycleTracks 是 semantic-converged membership change 的 hot path。
// 前一份 desired 已全部 known、且沒有 pending departed 時，known map 與 dense tracks 表達的是
// 同一份關係集合；因此可直接複製舊 tracks，再和新 sorted visible 做一次線性 merge：
// retained Entity 直接保留 dense history、removed Entity 進 departed、新 Entity 為 unknown。
// 整段不需要逐 Entity 查 known / generation / sent-build maps。
func rebuildConvergedLifecycleTracks(state *viewState, frame *simulation.ReplicationFrame, visibleIndices []int) {
	var oldTracks []entityTrack
	if pooled := lifecycleTrackScratchPool.Get(); pooled != nil {
		oldTracks = pooled.([]entityTrack)
	}
	if cap(oldTracks) < len(state.tracks) {
		oldTracks = make([]entityTrack, len(state.tracks))
	} else {
		oldTracks = oldTracks[:len(state.tracks)]
	}
	copy(oldTracks, state.tracks)
	defer func() {
		if cap(oldTracks) <= 4096 {
			lifecycleTrackScratchPool.Put(oldTracks[:0])
		}
	}()

	state.departed = state.departed[:0]
	count := len(visibleIndices)
	state.desiredIDs = resizeEntityIDs(state.desiredIDs, count)
	state.tracks = resizeEntityTracks(state.tracks, count)

	oldIndex := 0
	for newIndex, frameIndex := range visibleIndices {
		if frameIndex < 0 || frameIndex >= len(frame.Entities) {
			continue
		}
		newID := frame.Entities[frameIndex].ID
		for oldIndex < len(oldTracks) && oldTracks[oldIndex].id < newID {
			if oldTracks[oldIndex].known {
				state.departed = append(state.departed, oldTracks[oldIndex].id)
			}
			oldIndex++
		}
		state.desiredIDs[newIndex] = newID
		if oldIndex < len(oldTracks) && oldTracks[oldIndex].id == newID {
			state.tracks[newIndex] = oldTracks[oldIndex]
			oldIndex++
			continue
		}
		state.tracks[newIndex] = entityTrack{id: newID}
	}
	for oldIndex < len(oldTracks) {
		if oldTracks[oldIndex].known {
			state.departed = append(state.departed, oldTracks[oldIndex].id)
		}
		oldIndex++
	}
}

// rebuildLifecycleDesiredTracks 是 lifecycle-first membership-change 的 conservative path。
// 對 retained/known Entity，必須保留 lastDeliveredGeneration / lastSentBuild；對新進 AOI、
// 尚未成功 Spawn 的 Entity，history 在 known=false 期間不可觀察，因此避免兩個無效 map lookup。
func rebuildLifecycleDesiredTracks(state *viewState, frame *simulation.ReplicationFrame, visibleIndices []int) {
	count := len(visibleIndices)
	state.desiredIDs = resizeEntityIDs(state.desiredIDs, count)
	state.tracks = resizeEntityTracks(state.tracks, count)
	for i, index := range visibleIndices {
		if index < 0 || index >= len(frame.Entities) {
			continue
		}
		id := frame.Entities[index].ID
		_, known := state.known[id]
		track := entityTrack{id: id, known: known}
		if known {
			track.lastDeliveredGeneration = state.lastDeliveredGeneration[id]
			track.lastSentBuild = state.lastSentBuild[id]
		}
		state.desiredIDs[i] = id
		state.tracks[i] = track
	}
}
