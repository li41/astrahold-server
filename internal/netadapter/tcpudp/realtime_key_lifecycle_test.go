package tcpudp

import (
	"net"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterPeerRejectsRealtimeCredentialCollisionsWithoutOverwrite(t *testing.T) {
	server := NewServer(DefaultConfig(), newFakeRuntime(), gamev1.Codec{})

	firstToken := Token{1, 2, 3, 4}
	first := &peer{sessionID: 1, entityID: 1, token: firstToken, route: firstToken.RoutingID()}
	if err := server.registerPeer(first); err != nil {
		t.Fatal(err)
	}

	tokenCollision := &peer{sessionID: 2, entityID: 2, token: firstToken, route: firstToken.RoutingID()}
	if err := server.registerPeer(tokenCollision); err != ErrRealtimeCredentialCollision {
		t.Fatalf("token collision error=%v want=%v", err, ErrRealtimeCredentialCollision)
	}
	if server.peers[firstToken] != first || server.routes[first.route] != first {
		t.Fatal("token collision overwrote the existing peer")
	}

	secondToken := Token{9, 8, 7, 6}
	routeCollision := &peer{sessionID: 3, entityID: 3, token: secondToken, route: first.route}
	if err := server.registerPeer(routeCollision); err != ErrRealtimeCredentialCollision {
		t.Fatalf("route collision error=%v want=%v", err, ErrRealtimeCredentialCollision)
	}
	if server.peers[secondToken] != nil || server.peers[firstToken] != first || server.routes[first.route] != first {
		t.Fatal("route collision partially published or overwrote a peer")
	}
}

func TestRevokePeerRealtimeRemovesOnlyMatchingGeneration(t *testing.T) {
	server := NewServer(DefaultConfig(), newFakeRuntime(), gamev1.Codec{})
	token := Token{5, 4, 3, 2, 1}
	old := &peer{sessionID: 1, entityID: 7, token: token, route: token.RoutingID()}
	if err := server.registerPeer(old); err != nil {
		t.Fatal(err)
	}
	server.revokePeerRealtime(old)
	if server.peers[token] != nil || server.routes[old.route] != nil {
		t.Fatal("revoked generation remained routable")
	}

	replacement := &peer{sessionID: 2, entityID: 7, token: token, route: old.route}
	server.mu.Lock()
	server.peers[token] = replacement
	server.routes[old.route] = replacement
	server.mu.Unlock()
	server.revokePeerRealtime(old)
	if server.peers[token] != replacement || server.routes[old.route] != replacement {
		t.Fatal("stale revoke erased a replacement generation")
	}
}

func TestClosePeerRevokesRealtimeRouteBeforeReturn(t *testing.T) {
	server := NewServer(DefaultConfig(), newFakeRuntime(), gamev1.Codec{})
	token := Token{7, 7, 7, 7}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	connection := newClientConnection(serverSide, nil, token, gamev1.Codec{}, 8, &server.metrics)
	p := &peer{sessionID: session.ID(11), entityID: world.EntityID(21), token: token, route: token.RoutingID(), conn: connection}
	if err := server.registerPeer(p); err != nil {
		t.Fatal(err)
	}
	p.ready.Store(true)

	server.closePeer(p, "test_revoke", nil)
	if p.ready.Load() {
		t.Fatal("closed generation remained ready")
	}
	if server.peers[token] != nil || server.routes[p.route] != nil {
		t.Fatal("closed generation remained registered")
	}
}
