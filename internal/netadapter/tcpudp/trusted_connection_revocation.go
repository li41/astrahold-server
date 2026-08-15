package tcpudp

import "errors"

var ErrTrustedCharacterAuthenticationScopeRevoked = errors.New("tcpudp: trusted character authentication scope revoked")

// ReplaceTrustedCharacterAuthenticationScopes atomically replaces the set of trusted
// authentication proof generations that may publish or remain as live peers.
//
// Calling this method enables the fail-closed scope policy for trusted authenticated
// connections. For a peer that is no longer allowed, the same Server mutex section publishes
// ready=false and removes both realtime lookup handles before the policy update becomes visible
// to other transport goroutines. TCP close and fenced world leave then run outside the mutex
// through closePeer.
//
// The publication check in registerPeer uses the same policy under s.mu, so an in-flight
// authentication result produced before a credential reload cannot race past a later scope
// replacement and become authoritative after its credential generation was retired.
func (s *Server) ReplaceTrustedCharacterAuthenticationScopes(scopes []string) int {
	if s == nil {
		return 0
	}
	next := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		next[scope] = struct{}{}
	}

	s.mu.Lock()
	s.trustedScopePolicyEnabled = true
	s.trustedScopes = next
	retire := make([]*peer, 0)
	for _, p := range s.peers {
		if p == nil || !p.trustedAuthenticated {
			continue
		}
		_, allowed := next[p.trustedRevocationScope]
		if p.trustedRevocationScope != "" && allowed {
			continue
		}
		// Publish transport invalidation atomically with the policy replacement so
		// UDP lookup cannot observe a revoked peer after this lock is released.
		p.ready.Store(false)
		s.revokePeerRealtimeLocked(p)
		retire = append(retire, p)
	}
	s.mu.Unlock()

	for _, p := range retire {
		s.closePeer(p, "trusted_auth_scope_retire", nil)
	}
	return len(retire)
}
