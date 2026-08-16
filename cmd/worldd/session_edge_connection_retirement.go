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
	"F.20 edge-policy cutover fence: after a successful edge-policy reload, close trusted-proxy TLS connections authenticated by an older edge generation",
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
	if binding, ok := a.edgePolicy.connection(remote); ok && binding.generation != 0 {
		generation = binding.generation
	}
	tracker.connections[connection] = generation
	tracker.mu.Unlock()

	if generation == 0 {
		return
	}
	current := a.edgePolicy.Snapshot().Generation
	if current != 0 && generation < current {
		_ = connection.Close()
	}
}

// retireOldEdgeConnections closes every currently tracked trusted-proxy TLS
// connection whose authenticated F.19 generation predates currentGeneration.
// The caller invokes this only after a candidate edge policy has fully
// validated and published. Invalid reloads never enter this fence.
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
		if binding, ok := a.edgePolicy.connection(connection.RemoteAddr().String()); ok && binding.generation != 0 {
			generation = binding.generation
			tracker.connections[connection] = generation
		}
		if generation == 0 || generation >= currentGeneration {
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
