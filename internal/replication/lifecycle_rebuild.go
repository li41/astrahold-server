package replication

import "github.com/li41/astrahold-server/internal/simulation"

// rebuildLifecycleDesiredTracks 是 lifecycle-first membership-change 專用的 dense-track rebuild。
//
// 對 retained/known Entity，必須保留 lastDeliveredGeneration / lastSentBuild，因為 lifecycle
// 收斂後同一個 build 可能立即回到 realtime snapshot scheduler。對新進 AOI、尚未成功
// Spawn 的 Entity，這兩個 history 在 known=false 期間完全不可觀察：Spawn 本身已帶 authoritative
// transform，且 unknown Entity 不會進 snapshot candidate。因此避免兩個無效 map lookup，可降低
// mass teleport/churn 時每 Session × new-visible 的 membership rebuild CPU，而不改 lifecycle truth。
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
