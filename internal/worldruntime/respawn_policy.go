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

// scheduleRespawnForDefeat binds context, checkpoint, authoritative destination and earliest due
// tick at the alive -> defeated transition. Protocol v19 changes only the final release condition:
// the schedule becomes executable after due only when the owning client has requested restart.
func (r *Runtime) scheduleRespawnForDefeat(entityID world.EntityID, tick uint64, context respawnpolicy.DeathContext, report *StepReport) (respawnpolicy.Scheduled, bool) {
	if r.respawnPolicy == nil {
		return respawnpolicy.Scheduled{}, false
	}
	scheduled, err := r.respawnPolicy.Schedule(entityID, tick, context)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "schedule_respawn", Err: err})
		return respawnpolicy.Scheduled{}, false
	}
	report.Metrics.RespawnsScheduled++
	return scheduled, true
}

// classifyDeathContext only uses Server-owned authoritative entity kind.
func classifyDeathContext(actor, target world.EntityState) respawnpolicy.DeathContext {
	if actor.Kind == world.EntitySiegeObject {
		return respawnpolicy.DeathContextSiege
	}
	if actor.Kind == world.EntityPlayer && target.Kind == world.EntityPlayer {
		return respawnpolicy.DeathContextPvP
	}
	return respawnpolicy.DeathContextPvE
}

// applyDueRespawns runs after command handling and before simulation. A due schedule is necessary but
// no longer sufficient: Protocol v19 also requires restart consent from ClientRespawnRequest. This
// keeps the defeated session connected for a death modal while preserving Server-owned PvE/PvP/Siege
// delay and destination. An early click arms consent and is applied exactly when the policy becomes due.
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
			delete(r.respawnVitalsPhases, scheduled.EntityID)
			continue
		}
		if r.respawnVitalsPhases[scheduled.EntityID] != respawnVitalsRestartRequested {
			continue
		}
		r.applyRespawn("respawn_policy", RespawnRequest{EntityID: scheduled.EntityID, Position: scheduled.Position}, report)
	}
}
