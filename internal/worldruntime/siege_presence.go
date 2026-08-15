package worldruntime

import (
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

// observeSiegeThronePresence samples only authoritative post-simulation state from the
// same stable session list already used by Siege replication. Team ownership stays in
// siege.Service; positions and defeated state come from Server world/character state.
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
