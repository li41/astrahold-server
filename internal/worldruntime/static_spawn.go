package worldruntime

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidSpawnEntityRequest = errors.New("worldruntime: invalid spawn entity request")

// SpawnEntityRequest describes a server-owned non-session combat entity. It is deliberately
// registration-only: the entity enters the existing simulation, character, AOI and combat paths;
// it does not create a second spawn or authority pipeline.
type SpawnEntityRequest struct {
	Entity        world.EntityState
	Speed         float32
	Radius        float32
	MaxStepHeight float32
	HP            uint32
	MaxHP         uint32
}

func validateSpawnEntityRequest(request SpawnEntityRequest) error {
	if request.Entity.ID == 0 || !combatantKind(request.Entity.Kind) || request.Entity.Kind == world.EntityPlayer {
		return ErrInvalidSpawnEntityRequest
	}
	if request.Entity.ArchetypeID == "" || request.HP == 0 || request.MaxHP == 0 || request.HP > request.MaxHP {
		return ErrInvalidSpawnEntityRequest
	}
	for _, value := range []float32{request.Speed, request.Radius, request.MaxStepHeight} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 {
			return ErrInvalidSpawnEntityRequest
		}
	}
	if request.Radius == 0 {
		return ErrInvalidSpawnEntityRequest
	}
	return nil
}

// EnqueueSpawnEntity crosses the same world-owner queue used by player joins and gameplay
// commands. The request is applied only by Runtime.Step.
func (r *Runtime) EnqueueSpawnEntity(request SpawnEntityRequest) error {
	if err := validateSpawnEntityRequest(request); err != nil {
		return err
	}
	return r.queue.tryPush(spawnEntityCommand{request: request})
}

func (r *Runtime) applySpawnEntity(name string, request SpawnEntityRequest, report *StepReport) {
	if err := r.world.Spawn(request.Entity, request.Speed, request.Radius, request.MaxStepHeight); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	if err := r.characters.RegisterState(character.State{
		EntityID: request.Entity.ID,
		HP:       request.HP,
		MaxHP:    request.MaxHP,
	}); err != nil {
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	r.ensureEntityVitalsRevision(request.Entity.ID)
}
