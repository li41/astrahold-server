package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

// RespawnRequest only accepts an authoritative destination produced by server-side gameplay/admin
// policy. Protocol v19 ClientRespawnRequest carries no position and can only arm this transition.
type RespawnRequest struct {
	EntityID world.EntityID
	Position world.Position
}

type respawnCommand struct{ request RespawnRequest }

func (respawnCommand) name() string { return "respawn_character" }

// respawnVitalsPhase is the per-Runtime respawn transition barrier. RestartRequested is a
// pre-respawn consent phase; the remaining phases protect post-respawn AOI/vitals ordering.
type respawnVitalsPhase uint8

const (
	respawnVitalsRestartRequested respawnVitalsPhase = iota + 1
	respawnVitalsAwaitingAOI
	respawnVitalsDesiredOnly
)

// EnqueueRespawn keeps the invariant that all mutable world state enters through the bounded queue.
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

	// Teleport is the authoritative gameplay-transition primitive: it updates transform, movement
	// position and spatial index and clears old movement direction. Character state is already
	// preflighted as Defeated on this same owner goroutine.
	if err := r.world.Teleport(request.EntityID, request.Position); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	if _, err := r.characters.ReviveFull(request.EntityID); err != nil {
		// Only an invariant violation should reach this branch. Best-effort restore the old position
		// so vitals and transform do not intentionally diverge.
		_ = r.world.Teleport(request.EntityID, previous.Transform.Position)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}

	// Server/admin respawn and policy due share this primitive. Success always consumes the old
	// schedule so the same death cannot respawn twice.
	if r.respawnPolicy != nil {
		r.respawnPolicy.Cancel(request.EntityID)
	}

	// Protection is server-side damage legality state and does not alter AOI/vitals ordering.
	r.grantReviveProtection(request.EntityID, report.Tick, report)

	// Respawn changes vitals and AOI position together. Keep dirty vitals, but suppress fan-out until
	// the next normal snapshot rebuilds desired membership so stale-known observers cannot see revive.
	r.markEntityVitalsDirty(request.EntityID)
	r.respawnVitalsPhases[request.EntityID] = respawnVitalsAwaitingAOI
	report.Metrics.RespawnsApplied++
}

// reconcileRespawnVitalsAfterSnapshot runs only after a complete normal snapshot pass. A pending
// restart request must survive snapshots while the character is still defeated. Post-respawn phases
// continue to guard stale known relationships until lifecycle convergence is confirmed.
func (r *Runtime) reconcileRespawnVitalsAfterSnapshot() {
	for entityID, phase := range r.respawnVitalsPhases {
		switch phase {
		case respawnVitalsRestartRequested:
			// Consent remains armed until authoritative policy becomes due and applyRespawn replaces it.
			continue
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
