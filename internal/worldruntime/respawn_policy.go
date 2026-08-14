package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrRespawnPolicyUnavailable = errors.New("worldruntime: respawn policy unavailable")

type acquireRespawnCheckpointCommand struct {
	entityID     world.EntityID
	spawnPointID string
}

func (acquireRespawnCheckpointCommand) name() string { return "acquire_respawn_checkpoint" }

type setRespawnCheckpointCommand = acquireRespawnCheckpointCommand

func WithRespawnPolicy(policy *respawnpolicy.Service) Option {
	return func(r *Runtime) { r.respawnPolicy = policy }
}

// EnqueueAcquireRespawnCheckpoint 是 server-owned gameplay seam。Client不提供 position；
// WorldRuntime在 owner tick 使用 authoritative entity position驗證 checkpoint activation。
func (r *Runtime) EnqueueAcquireRespawnCheckpoint(entityID world.EntityID, spawnPointID string) error {
	return r.queue.tryPush(acquireRespawnCheckpointCommand{entityID: entityID, spawnPointID: spawnPointID})
}

// EnqueueSetRespawnCheckpoint 保留 S3-F.3 server-side API名稱，但非空值現在走 acquisition validity，
// 不再允許 gameplay caller遠端指定任意 checkpoint。空值等價於 Clear。
func (r *Runtime) EnqueueSetRespawnCheckpoint(entityID world.EntityID, spawnPointID string) error {
	if spawnPointID == "" {
		return r.EnqueueClearRespawnCheckpoint(entityID)
	}
	return r.EnqueueAcquireRespawnCheckpoint(entityID, spawnPointID)
}

func (r *Runtime) EnqueueClearRespawnCheckpoint(entityID world.EntityID) error {
	return r.queue.tryPush(acquireRespawnCheckpointCommand{entityID: entityID})
}

func (r *Runtime) applyAcquireRespawnCheckpoint(name string, command acquireRespawnCheckpointCommand, report *StepReport) {
	if r.respawnPolicy == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: ErrRespawnPolicyUnavailable})
		return
	}
	state, ok := r.characters.State(command.entityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: character.ErrCharacterNotFound})
		return
	}
	if command.spawnPointID == "" {
		r.respawnPolicy.ClearCheckpoint(command.entityID)
		return
	}
	if state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: character.ErrCharacterDefeated})
		return
	}
	entity, ok := r.world.Entity(command.entityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: simulation.ErrEntityNotFound})
		return
	}
	if err := r.respawnPolicy.AcquireCheckpoint(command.entityID, entity.Transform.Position, command.spawnPointID); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
}

func (r *Runtime) applySetRespawnCheckpoint(name string, command setRespawnCheckpointCommand, report *StepReport) {
	r.applyAcquireRespawnCheckpoint(name, command, report)
}

// scheduleRespawnForDefeat 只在 Character alive -> defeated transition 成功後呼叫。
// Policy 在死亡當下綁定 context、checkpoint與 due tick；排程失敗不回滾已成立的 combat damage。
func (r *Runtime) scheduleRespawnForDefeat(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) {
	if r.respawnPolicy == nil {
		return
	}
	if _, err := r.respawnPolicy.Schedule(entityID, tick, context); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "schedule_respawn", Err: err})
		return
	}
	report.Metrics.RespawnsScheduled++
}

// classifyDeathContext 只使用 Server-owned authoritative entity kind。
// 若未來 Siege mode要把 player-vs-player改判為 Siege，可在更上層 source resolver接入，
// 不需要增加 Client death-context欄位。
func classifyDeathContext(actor, target world.EntityState) respawnpolicy.DeathContext {
	if actor.Kind == world.EntitySiegeObject {
		return respawnpolicy.DeathContextSiege
	}
	if actor.Kind == world.EntityPlayer && target.Kind == world.EntityPlayer {
		return respawnpolicy.DeathContextPvP
	}
	return respawnpolicy.DeathContextPvE
}

// applyDueRespawns 在 command phase 完成後、simulation Tick 前執行。
// 同一 due tick 收到的 ClientMoveInput 仍先以 Defeated 規則 consume/zero；角色復活後必須等新的 input。
// Due selection 本身不移除 pending：authoritative transition成功時由 applyRespawn Cancel；若 transition fault，
// pending會留到下一 tick重試。只有 entity不存在或已由其他合法路徑復活時才視為 stale schedule並清除。
func (r *Runtime) applyDueRespawns(tick uint64, report *StepReport) {
	if r.respawnPolicy == nil {
		return
	}
	due := r.respawnPolicy.Due(tick)
	report.Metrics.RespawnPolicyDue += len(due)
	for _, scheduled := range due {
		state, ok := r.characters.State(scheduled.EntityID)
		if !ok || !state.Defeated {
			r.respawnPolicy.Cancel(scheduled.EntityID)
			continue
		}
		r.applyRespawn("respawn_policy", RespawnRequest{EntityID: scheduled.EntityID, Position: scheduled.Position}, report)
	}
}
