package replication

import (
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

// Wants 回報 entity 是否位於該 Session 最近一次 replication build 的 desired AOI。
//
// 這和 Knows 不同：Knows 是 Reliable Spawn 已確認的 lifecycle knowledge；Wants 是
// 最近一次 AOI membership truth。S3-F.2 只在 respawn transition 的短暫 ordering barrier
// 使用這個查詢，不把它放回一般 Dirty Vitals hot path。
func (s *Service) Wants(sessionID session.ID, entityID world.EntityID) bool {
	state := s.views[sessionID]
	if state == nil {
		return false
	}
	return desiredIndex(state.desiredIDs, entityID) >= 0
}

// HasKnownOutsideDesired 檢查是否仍有 Session 已知 entity、但最近 desired AOI 已不包含它。
// 這通常代表 EntityDespawn 尚未成功確認。Respawn Vitals 在這段期間只可送給 Wants=true
// 的 Session，避免舊 AOI observer 在 Despawn backpressure 時先收到復活狀態。
func (s *Service) HasKnownOutsideDesired(entityID world.EntityID) bool {
	for _, state := range s.views {
		if state == nil {
			continue
		}
		if _, known := state.known[entityID]; !known {
			continue
		}
		if desiredIndex(state.desiredIDs, entityID) < 0 {
			return true
		}
	}
	return false
}
