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

// reviveProtectionUntilTick 回傳目前有效的 Server-owned protection end tick。
// 自然到期採 lazy expiry；Client 只能顯示這個 tick，不能把它當成自己的 gameplay timer。
func (r *Runtime) reviveProtectionUntilTick(entityID world.EntityID, tick uint64) uint64 {
	until, ok := r.reviveProtectionUntil[entityID]
	if !ok {
		return 0
	}
	if tick >= until {
		delete(r.reviveProtectionUntil, entityID)
		return 0
	}
	return until
}

// isReviveProtected 以 lazy expiry 避免每 tick 對所有 player 做 O(N) sweep。
func (r *Runtime) isReviveProtected(entityID world.EntityID, tick uint64) bool {
	return r.reviveProtectionUntilTick(entityID, tick) != 0
}

// cancelReviveProtectionByDamageAction 只有成功造成 damage effect 才取消 grace。
// range/LOS/cooldown/target-protected 等 rejection 都不會取消。實際提前取消時標記 Vitals dirty，
// 讓 observers 收到 until=0，而不是在 Client 端猜測取消時機。
func (r *Runtime) cancelReviveProtectionByDamageAction(entityID world.EntityID, report *StepReport) {
	if _, ok := r.reviveProtectionUntil[entityID]; !ok {
		return
	}
	delete(r.reviveProtectionUntil, entityID)
	r.markEntityVitalsDirty(entityID)
	if report != nil {
		report.Metrics.ReviveProtectionsCancelledByDamageAction++
	}
}

func (r *Runtime) clearReviveProtection(entityID world.EntityID) {
	delete(r.reviveProtectionUntil, entityID)
}
