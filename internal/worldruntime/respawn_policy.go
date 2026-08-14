package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrRespawnPolicyUnavailable = errors.New("worldruntime: respawn policy unavailable")

type setRespawnCheckpointCommand struct {
	entityID     world.EntityID
	spawnPointID string
}

func (setRespawnCheckpointCommand) name() string { return "set_respawn_checkpoint" }

func WithRespawnPolicy(policy *respawnpolicy.Service) Option {
	return func(r *Runtime) { r.respawnPolicy = policy }
}

// EnqueueSetRespawnCheckpoint 是 server-side gameplay / admin seam；空 spawnPointID 代表回復 default policy。
// Client protocol 本階段仍不能直接指定 checkpoint 或 respawn destination。
func (r *Runtime) EnqueueSetRespawnCheckpoint(entityID world.EntityID, spawnPointID string) error {
	return r.queue.tryPush(setRespawnCheckpointCommand{entityID: entityID, spawnPointID: spawnPointID})
}

func (r *Runtime) applySetRespawnCheckpoint(name string, command setRespawnCheckpointCommand, report *StepReport) {
	if r.respawnPolicy == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: ErrRespawnPolicyUnavailable})
		return
	}
	if _, ok := r.characters.State(command.entityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: character.ErrCharacterNotFound})
		return
	}
	if command.spawnPointID == "" {
		r.respawnPolicy.ClearCheckpoint(command.entityID)
		return
	}
	if err := r.respawnPolicy.SetCheckpoint(command.entityID, command.spawnPointID); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
	}
}

// scheduleRespawnForDefeat 只在 Character alive -> defeated transition 成功後呼叫。
// Policy 在死亡當下綁定 checkpoint 與 due tick；排程失敗不回滾已成立的 combat damage。
func (r *Runtime) scheduleRespawnForDefeat(entityID world.EntityID, tick uint64, report *StepReport) {
	if r.respawnPolicy == nil {
		return
	}
	if _, err := r.respawnPolicy.Schedule(entityID, tick); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "schedule_respawn", Err: err})
		return
	}
	report.Metrics.RespawnsScheduled++
}

// applyDueRespawns 在 command phase 完成後、simulation Tick 前執行。
// 同一 due tick 收到的 ClientMoveInput 仍先以 Defeated 規則 consume/zero；角色復活後必須等新的 input。
func (r *Runtime) applyDueRespawns(tick uint64, report *StepReport) {
	if r.respawnPolicy == nil {
		return
	}
	due := r.respawnPolicy.Due(tick)
	report.Metrics.RespawnPolicyDue += len(due)
	for _, scheduled := range due {
		state, ok := r.characters.State(scheduled.EntityID)
		if !ok || !state.Defeated {
			continue
		}
		r.applyRespawn("respawn_policy", RespawnRequest{EntityID: scheduled.EntityID, Position: scheduled.Position}, report)
	}
}
