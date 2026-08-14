package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

// RespawnRequest 只接受 server-side gameplay / admin policy 產生的 authoritative 目的地。
// Client protocol 不提供 respawn position，也不在 S3-F.2 新增 ClientRespawn message。
type RespawnRequest struct {
	EntityID world.EntityID
	Position world.Position
}

type respawnCommand struct{ request RespawnRequest }

func (respawnCommand) name() string { return "respawn_character" }

type respawnVitalsPhase uint8

const (
	respawnVitalsAwaitingAOI respawnVitalsPhase = iota + 1
	respawnVitalsDesiredOnly
)

// EnqueueRespawn 保持「所有 world mutable state 只能由 bounded command queue 進入」的不變量。
func (r *Runtime) EnqueueRespawn(request RespawnRequest) error {
	return r.queue.tryPush(respawnCommand{request: request})
}

func (r *Runtime) applyRespawn(name string, request RespawnRequest, report *StepReport) {
	state, ok := r.characters.State(request.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: character.ErrCharacterNotFound})
		return
	}
	if !state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: character.ErrCharacterNotDefeated})
		return
	}

	previous, ok := r.world.Entity(request.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: simulation.ErrEntityNotFound})
		return
	}

	// Teleport 是 authoritative gameplay-transition primitive，會同步更新 transform、movement
	// position、spatial index，並清掉舊 movement direction。Character state 已在同一 owner
	// goroutine preflight 為 Defeated，因此後續 ReviveFull 不存在競爭式 state change。
	if err := r.world.Teleport(request.EntityID, request.Position); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	if _, err := r.characters.ReviveFull(request.EntityID); err != nil {
		// 理論上只有 invariant violation 才會走到這裡。盡力還原原 position，避免 vitals / transform
		// 分裂；rollback 本身若失敗也保留原錯誤作為主要 fault。
		_ = r.world.Teleport(request.EntityID, previous.Transform.Position)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}

	// 手動 server respawn 與 policy due 共用同一 primitive；成功後一律清掉舊 pending，
	// 避免之後同一 death schedule 再次觸發第二次 respawn。
	if r.respawnPolicy != nil {
		r.respawnPolicy.Cancel(request.EntityID)
	}

	// Protection 是純 server-side damage legality state；不需要改變 respawn AOI/Vitals ordering。
	r.grantReviveProtection(request.EntityID, report.Tick, report)

	// Respawn 同時改 vitals 與 AOI position。Dirty Vitals 先保留，但在下一次正常 snapshot
	// 完成 desired membership rebuild 前不得 fan-out，避免 stale-known observer 先看到復活狀態。
	r.markEntityVitalsDirty(request.EntityID)
	r.respawnVitalsPhases[request.EntityID] = respawnVitalsAwaitingAOI
	report.Metrics.RespawnsApplied++
}

// reconcileRespawnVitalsAfterSnapshot 只在完整 normal snapshot pass 後呼叫。
// 若仍有 known-but-no-longer-desired relationship（通常是 Despawn backpressure），
// respawn Vitals 進入 desired-only 模式；等 stale knowledge 全部確認清除後回到一般 hot path。
func (r *Runtime) reconcileRespawnVitalsAfterSnapshot() {
	for entityID, phase := range r.respawnVitalsPhases {
		switch phase {
		case respawnVitalsAwaitingAOI:
			if r.replication.HasKnownOutsideDesired(entityID) {
				r.respawnVitalsPhases[entityID] = respawnVitalsDesiredOnly
			} else {
				delete(r.respawnVitalsPhases, entityID)
			}
		case respawnVitalsDesiredOnly:
			if !r.replication.HasKnownOutsideDesired(entityID) {
				delete(r.respawnVitalsPhases, entityID)
			}
		default:
			delete(r.respawnVitalsPhases, entityID)
		}
	}
}
