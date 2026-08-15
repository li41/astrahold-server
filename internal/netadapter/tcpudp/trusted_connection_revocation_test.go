package tcpudp

import (
	"errors"
	"net"
	"testing"
)

func TestTrustedAuthenticationScopePolicyRejectsStaleInFlightPeer(t *testing.T) {
	s := &Server{
		peers:  make(map[Token]*peer),
		routes: make(map[Token]*peer),
	}
	if retired := s.ReplaceTrustedCharacterAuthenticationScopes([]string{"scope-new"}); retired != 0 {
		t.Fatalf("retired=%d want=0", retired)
	}

	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	p := &peer{
		token:                  token,
		route:                  token.RoutingID(),
		trustedAuthenticated:   true,
		trustedRevocationScope: "scope-old",
	}
	if err := s.registerPeer(p); !errors.Is(err, ErrTrustedCharacterAuthenticationScopeRevoked) {
		t.Fatalf("register err=%v want=%v", err, ErrTrustedCharacterAuthenticationScopeRevoked)
	}
	if len(s.peers) != 0 || len(s.routes) != 0 {
		t.Fatalf("revoked in-flight peer was published: peers=%d routes=%d", len(s.peers), len(s.routes))
	}
}

func TestReplaceTrustedAuthenticationScopesRetiresExistingPeerAndRevokesRealtimeLookup(t *testing.T) {
	s := &Server{
		peers:  make(map[Token]*peer),
		routes: make(map[Token]*peer),
	}
	s.ReplaceTrustedCharacterAuthenticationScopes([]string{"scope-old"})

	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	p := &peer{
		sessionID:              1,
		token:                  token,
		route:                  token.RoutingID(),
		conn:                   newClientConnection(serverConn, nil, token, nil, 1, &s.metrics),
		trustedAuthenticated:   true,
		trustedRevocationScope: "scope-old",
	}
	p.ready.Store(true)
	if err := s.registerPeer(p); err != nil {
		t.Fatal(err)
	}

	if retired := s.ReplaceTrustedCharacterAuthenticationScopes([]string{"scope-new"}); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if p.ready.Load() {
		t.Fatal("retired peer must become realtime-not-ready before teardown")
	}
	if len(s.peers) != 0 || len(s.routes) != 0 {
		t.Fatalf("retired peer realtime lookup remains: peers=%d routes=%d", len(s.peers), len(s.routes))
	}
	select {
	case <-p.conn.done:
	default:
		t.Fatal("retired trusted peer connection was not closed")
	}
}

func TestTrustedAuthenticationScopePolicyPreservesAllowedAndUnauthenticatedPeers(t *testing.T) {
	s := &Server{
		peers:  make(map[Token]*peer),
		routes: make(map[Token]*peer),
	}
	s.ReplaceTrustedCharacterAuthenticationScopes([]string{"scope-live"})

	trustedToken, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	trusted := &peer{
		token:                  trustedToken,
		route:                  trustedToken.RoutingID(),
		trustedAuthenticated:   true,
		trustedRevocationScope: "scope-live",
	}
	if err := s.registerPeer(trusted); err != nil {
		t.Fatal(err)
	}

	devToken, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	dev := &peer{token: devToken, route: devToken.RoutingID()}
	if err := s.registerPeer(dev); err != nil {
		t.Fatal(err)
	}

	if retired := s.ReplaceTrustedCharacterAuthenticationScopes([]string{"scope-live"}); retired != 0 {
		t.Fatalf("retired=%d want=0", retired)
	}
	if s.peers[trustedToken] != trusted || s.peers[devToken] != dev {
		t.Fatal("allowed trusted peer or unauthenticated development peer was removed")
	}
}
