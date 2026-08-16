package main

import (
	"flag"
	"net"
	"net/http"
	"sync"
)

var sessionLoginTrustedProxyEdgeRetireOldConnections = flag.Bool(
	"session-login-trusted-proxy-edge-retire-old-connections",
	false,
	"F.20/F.22/F.23/F.24 edge authority cutover fence: retire stale or revoked trusted-proxy TLS authority after successful policy/revocation reloads while preserving compatible healthy credentials",
)

type sessionEdgeTrackedConnection struct {
	connection net.Conn
	generation uint64
}

type sessionEdgeConnectionTracker struct {
	mu          sync.Mutex
	connections map[net.Conn]uint64
}

var sessionEdgeConnectionTrackers sync.Map // map[*sessionSourceAttributor]*sessionEdgeConnectionTracker

func sessionEdgeConnectionRetirementRequested() bool {
	return sessionLoginTrustedProxyEdgeRetireOldConnections != nil && *sessionLoginTrustedProxyEdgeRetireOldConnections
}

func (a *sessionSourceAttributor) edgeConnectionRetirementEnabled() bool {
	return a != nil && a.edgePolicy != nil && sessionEdgeConnectionRetirementRequested()
}

func (a *sessionSourceAttributor) edgeConnectionCutoverMode() string {
	if a != nil && a.edgePolicy != nil && a.edgeConnectionRetirementEnabled() {
		return "retire-old"
	}
	if a != nil && a.edgePolicy != nil {
		return "graceful"
	}
	return "none"
}

func (a *sessionSourceAttributor) edgeConnectionTracker() *sessionEdgeConnectionTracker {
	if !a.edgeConnectionRetirementEnabled() {
		return nil
	}
	if tracker, ok := sessionEdgeConnectionTrackers.Load(a); ok {
		return tracker.(*sessionEdgeConnectionTracker)
	}
	tracker := &sessionEdgeConnectionTracker{connections: make(map[net.Conn]uint64)}
	actual, _ := sessionEdgeConnectionTrackers.LoadOrStore(a, tracker)
	return actual.(*sessionEdgeConnectionTracker)
}

func (a *sessionSourceAttributor) observeEdgeConnectionState(connection net.Conn, state http.ConnState) {
	if a == nil || connection == nil || connection.RemoteAddr() == nil {
		return
	}
	remote := connection.RemoteAddr().String()
	if state == http.StateClosed || state == http.StateHijacked {
		a.releaseConnection(remote)
		if tracker, ok := sessionEdgeConnectionTrackers.Load(a); ok {
			tracked := tracker.(*sessionEdgeConnectionTracker)
			tracked.mu.Lock()
			delete(tracked.connections, connection)
			tracked.mu.Unlock()
		}
		return
	}
	tracker := a.edgeConnectionTracker()
	if tracker == nil {
		return
	}

	tracker.mu.Lock()
	generation := tracker.connections[connection]
	binding, authenticated := a.edgePolicy.connection(remote)
	if authenticated && binding.generation != 0 {
		generation = binding.generation
	}
	tracker.connections[connection] = generation
	tracker.mu.Unlock()

	if authenticated && a.edgePolicy.connectionRequiresRetirement(remote, binding) {
		_ = connection.Close()
		return
	}
	if generation == 0 {
		return
	}
	current := a.edgePolicy.Snapshot().Generation
	if current != 0 && generation < current && !authenticated {
		_ = connection.Close()
	}
}

// retireOldEdgeConnections applies the F.23 authenticated-identity fence to
// every currently tracked proxy connection older than the current generation.
// Global forwarding-mode, CA-root-set, or trusted-prefix topology changes still
// retire all older proxy connections. For identity-only changes, each connection
// survives only when at least one exact DNS identity authorized at its original
// handshake remains allowed for its current peer binding. Invalid reloads never
// enter this fence, and F.21 no-ops keep the generation unchanged.
func (a *sessionSourceAttributor) retireOldEdgeConnections(currentGeneration uint64) int {
	if !a.edgeConnectionRetirementEnabled() {
		return 0
	}
	tracker := a.edgeConnectionTracker()
	if tracker == nil {
		return 0
	}

	tracker.mu.Lock()
	retire := make([]sessionEdgeTrackedConnection, 0)
	for connection, generation := range tracker.connections {
		if connection == nil || connection.RemoteAddr() == nil {
			continue
		}
		remote := connection.RemoteAddr().String()
		binding, authenticated := a.edgePolicy.connection(remote)
		if authenticated && binding.generation != 0 {
			generation = binding.generation
			tracker.connections[connection] = generation
		}
		if authenticated {
			if !a.edgePolicy.connectionRequiresRetirement(remote, binding) {
				continue
			}
		} else if generation == 0 || currentGeneration == 0 || generation >= currentGeneration {
			continue
		}
		retire = append(retire, sessionEdgeTrackedConnection{connection: connection, generation: generation})
		delete(tracker.connections, connection)
	}
	tracker.mu.Unlock()

	for _, tracked := range retire {
		_ = tracked.connection.Close()
	}
	return len(retire)
}
