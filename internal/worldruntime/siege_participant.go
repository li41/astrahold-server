package worldruntime

import "github.com/li41/astrahold-server/internal/session"

func (r *Runtime) assignSiegeParticipant(s *session.Session) (bool, error) {
	if r == nil || r.siege == nil || s == nil {
		return false, nil
	}
	// Gate-only runtimes predate configured Siege matches and remain valid. Trusted roster
	// assignment is a match feature, not a prerequisite for the Gate combat service.
	if _, ok := r.siege.MatchState(); !ok {
		return false, nil
	}
	return r.siege.AssignResolvedParticipant(s.EntityID, s.CharacterIdentity)
}

func (r *Runtime) removeSiegeParticipant(s *session.Session) {
	if r == nil || r.siege == nil || s == nil {
		return
	}
	r.siege.RemoveParticipant(s.EntityID)
}
