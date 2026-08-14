package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/world"
)

var ErrEntityReviveProtected = errors.New("worldruntime: entity revive protected")

// grantReviveProtection 在成功 respawn / resurrection 後建立半開區間 [tick, until)。
// duration=0 代表此 Runtime fixture / environment 未啟用 protection policy。
func (r *Runtime) grantReviveProtection(entityID world.EntityID, tick uint64, report *StepReport) {
	duration := r.config.PostReviveProtectionTicks
	if entityID == 0 || duration == 0 {
		return
	}
	until := tick + duration
	if until < tick {
		until = ^uint64(0)
	}
	r.reviveProtectionUntil[entityID] = until
	if report != nil {
		report.Metrics.ReviveProtectionsGranted++
	}
}

// isReviveProtected 以 lazy expiry 避免每 tick 對所有 player 做 O(N) sweep。
func (r *Runtime) isReviveProtected(entityID world.EntityID, tick uint64) bool {
	until, ok := r.reviveProtectionUntil[entityID]
	if !ok {
		return false
	}
	if tick >= until {
		delete(r.reviveProtectionUntil, entityID)
		return false
	}
	return true
}

// cancelReviveProtectionByDamageAction 只有成功造成 damage effect 才取消 grace。
// range/LOS/cooldown/target-protected 等 rejection 都不會取消。
func (r *Runtime) cancelReviveProtectionByDamageAction(entityID world.EntityID, report *StepReport) {
	if _, ok := r.reviveProtectionUntil[entityID]; !ok {
		return
	}
	delete(r.reviveProtectionUntil, entityID)
	if report != nil {
		report.Metrics.ReviveProtectionsCancelledByDamageAction++
	}
}

func (r *Runtime) clearReviveProtection(entityID world.EntityID) {
	delete(r.reviveProtectionUntil, entityID)
}
