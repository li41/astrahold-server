package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

// updateSiegeObjectives runs after simulation from authoritative world/character state.
// It reuses the stable session list later consumed by SiegeMatchState replication.
func (r *Runtime) updateSiegeObjectives(sessions []*session.Session, delta time.Duration) {
	if r == nil || r.siege == nil {
		return
	}
	r.siegeStepDelta = delta
	state, ok := r.siege.MatchState()
	if !ok {
		return
	}
	if state.Phase == siege.MatchPhaseThrone {
		r.observeSiegeThronePresence(sessions)
	} else {
		r.siege.ObserveThronePresence(nil)
	}
	r.siege.AdvanceThroneCapture(delta)
	// D.3A resolves in the same post-simulation world-owner step that first reaches the
	// authoritative capture threshold. No Client message or later replication callback
	// decides victory or castle ownership.
	r.siege.ResolveThroneCapture()
}

// observeSiegeThronePresence samples only authoritative post-simulation state from the
// same stable session list used by Siege replication. Team ownership stays in siege.Service;
// positions and defeated state come from Server world/character state.
func (r *Runtime) observeSiegeThronePresence(sessions []*session.Session) bool {
	if r == nil || r.siege == nil {
		return false
	}
	observations := make([]siege.ParticipantPresence, 0, len(sessions))
	for _, s := range sessions {
		entity, ok := r.world.Entity(s.EntityID)
		if !ok {
			continue
		}
		characterState, ok := r.characters.State(s.EntityID)
		if !ok {
			continue
		}
		observations = append(observations, siege.ParticipantPresence{
			EntityID: s.EntityID,
			Position: entity.Transform.Position,
			Defeated: characterState.Defeated,
		})
	}
	return r.siege.ObserveThronePresence(observations)
}
