package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRebuildLifecycleDesiredTracksSkipsUnknownTransformHistory(t *testing.T) {
	svc := NewService()
	sid := session.ID(77)
	svc.Register(sid)
	state := svc.ensureView(sid)
	state.known[1] = struct{}{}
	state.lastDeliveredGeneration[1] = 7
	state.lastSentBuild[1] = 11
	// 模擬曾存在過、但目前 unknown 的 stale mirrors；lifecycle rebuild 不應把它們
	// 帶回 dense track，否則新 Spawn 前會做無意義 map reads，且可能污染 snapshot baseline。
	state.lastDeliveredGeneration[3] = 99
	state.lastSentBuild[3] = 101

	frame, visible := lifecycleFrameIDs(40, []world.EntityID{1, 3})
	rebuildLifecycleDesiredTracks(state, frame, visible)
	if len(state.tracks) != 2 {
		t.Fatalf("tracks=%d want=2", len(state.tracks))
	}
	retained := state.tracks[0]
	if !retained.known || retained.lastDeliveredGeneration != 7 || retained.lastSentBuild != 11 {
		t.Fatalf("retained track=%+v want known history 7/11", retained)
	}
	unknown := state.tracks[1]
	if unknown.known || unknown.lastDeliveredGeneration != 0 || unknown.lastSentBuild != 0 {
		t.Fatalf("unknown track=%+v want zero history", unknown)
	}

	batch := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, LifecycleLimits{MaxMessages: 2, MaxSpawns: 2, MaxDespawns: 2})
	if got := lifecycleIDs(batch, true); !equalIDs(got, []world.EntityID{3}) {
		t.Fatalf("spawn ids=%v want=[3]", got)
	}
	svc.ConfirmSpawn(sid, 3)

	// Spawn materialization 會以當下 authoritative generation 建立 baseline；確認後若 transform
	// 沒變，不應立刻再產生 realtime snapshot。
	next := svc.BuildFrameLifecycleFirst(sid, 1, 0, frame, visible, LifecycleLimits{MaxMessages: 2, MaxSpawns: 2, MaxDespawns: 2})
	if got := snapshotEntityIDs(next); len(got) != 0 {
		t.Fatalf("unchanged spawned entity repeated in snapshot: %v", got)
	}
}
