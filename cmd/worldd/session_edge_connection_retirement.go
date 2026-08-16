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
	"F.20/F.22 edge-policy cutover fence: retire stale trusted-proxy TLS authority after a successful edge-policy reload, preserving unaffected bindings when only their exact DNS identity mapping remains compatible",
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

	if generation == 0 {
		return
	}
	current := a.edgePolicy.Snapshot().Generation
	if current != 0 && generation < current {
		if !authenticated || a.edgePolicy.connectionRequiresRetirement(remote, binding) {
			_ = connection.Close()
		}
	}
}

// retireOldEdgeConnections applies the F.22 peer-specific compatibility fence
// to every currently tracked authenticated proxy connection older than the
// current generation. Global forwarding-mode, CA-root-set, or trusted-prefix
// topology changes retire all older proxy connections. Identity-only changes
// retire only peers whose own prefix->exact-DNS mapping changed. Invalid reloads
// never enter this fence, and F.21 no-ops keep the generation unchanged.
func (a *sessionSourceAttributor) retireOldEdgeConnections(currentGeneration uint64) int {
	if !a.edgeConnectionRetirementEnabled() || currentGeneration == 0 {
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
		if generation == 0 || generation >= currentGeneration {
			continue
		}
		if authenticated && !a.edgePolicy.connectionRequiresRetirement(remote, binding) {
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
