package tcpudp

import "errors"

var ErrRealtimeCredentialCollision = errors.New("tcpudp: realtime credential collision")

// registerPeer publishes the secret-keyed and public-route lookups atomically. A random
// token or derived-route collision must never replace an existing live transport: the new
// connection fails closed before world ownership is acquired.
func (s *Server) registerPeer(p *peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peers[p.token] != nil || s.routes[p.route] != nil {
		return ErrRealtimeCredentialCollision
	}
	s.peers[p.token] = p
	s.routes[p.route] = p
	return nil
}

// revokePeerRealtime removes both lookup handles for exactly this peer. Conditional deletion
// is important during takeover/close races: a stale peer can never erase a newer mapping.
func (s *Server) revokePeerRealtime(p *peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.peers[p.token]; current == p {
		delete(s.peers, p.token)
	}
	if current := s.routes[p.route]; current == p {
		delete(s.routes, p.route)
	}
}
