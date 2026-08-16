package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestSessionEdgeConnectionRetirementClosesOlderAuthenticatedGeneration(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.2", 6100)
	attributor.observeEdgeConnectionState(connection, http.StateNew)

	snapshot, generation := runtime.currentSnapshot()
	runtime.bindConnection(connection.RemoteAddr().String(), snapshot, generation, 0)
	attributor.observeEdgeConnectionState(connection, http.StateActive)

	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if !connection.Closed() {
		t.Fatal("older trusted-proxy connection was not closed")
	}
}

func TestSessionEdgeConnectionRetirementLeavesDirectConnectionOpen(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.1", 6200)
	attributor.observeEdgeConnectionState(connection, http.StateNew)

	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 0 {
		t.Fatalf("retired=%d want=0", retired)
	}
	if connection.Closed() {
		t.Fatal("direct Client connection was closed by edge retirement")
	}
}

func TestSessionEdgeConnectionRetirementGracefulCompatibilityMode(t *testing.T) {
	setSessionEdgeRetirementForTest(t, false)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.2", 6300)
	attributor.observeEdgeConnectionState(connection, http.StateNew)

	snapshot, generation := runtime.currentSnapshot()
	runtime.bindConnection(connection.RemoteAddr().String(), snapshot, generation, 0)
	attributor.observeEdgeConnectionState(connection, http.StateActive)
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 0 {
		t.Fatalf("retired=%d want=0", retired)
	}
	if connection.Closed() {
		t.Fatal("F.19 graceful compatibility connection was unexpectedly closed")
	}
}

func TestSessionEdgeConnectionRetirementInvalidReloadKeepsLKGConnection(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.2", 6400)
	attributor.observeEdgeConnectionState(connection, http.StateNew)

	snapshot, generation := runtime.currentSnapshot()
	runtime.bindConnection(connection.RemoteAddr().String(), snapshot, generation, 0)
	attributor.observeEdgeConnectionState(connection, http.StateIdle)
	if err := os.WriteFile(runtime.definitionFile, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("invalid replacement unexpectedly reloaded")
	}
	if connection.Closed() {
		t.Fatal("invalid edge-policy reload retired the last-known-good connection")
	}
	if metadata := runtime.Snapshot(); metadata.Generation != generation {
		t.Fatalf("generation=%d want=%d", metadata.Generation, generation)
	}
}

func TestSessionEdgeConnectionRetirementClosesHandshakeThatFinishesAfterCutover(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.2", 6500)
	attributor.observeEdgeConnectionState(connection, http.StateNew)

	oldSnapshot, oldGeneration := runtime.currentSnapshot()
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation <= oldGeneration {
		t.Fatalf("reload generation=%d old=%d", result.Generation, oldGeneration)
	}

	// This simulates a TLS handshake that selected generation 1 before reload,
	// but completed after generation 2 had already published.
	runtime.bindConnection(connection.RemoteAddr().String(), oldSnapshot, oldGeneration, 0)
	attributor.observeEdgeConnectionState(connection, http.StateActive)
	if !connection.Closed() {
		t.Fatal("stale handshake connection remained open after cutover")
	}
}

func setSessionEdgeRetirementForTest(t *testing.T, enabled bool) {
	t.Helper()
	previous := *sessionLoginTrustedProxyEdgeRetireOldConnections
	*sessionLoginTrustedProxyEdgeRetireOldConnections = enabled
	t.Cleanup(func() { *sessionLoginTrustedProxyEdgeRetireOldConnections = previous })
}

type sessionEdgeRetirementTestConn struct {
	remote net.Addr
	mu     sync.Mutex
	closed bool
}

func newSessionEdgeRetirementTestConn(ip string, port int) *sessionEdgeRetirementTestConn {
	return &sessionEdgeRetirementTestConn{remote: tcpAddr(ip, port)}
}

func (c *sessionEdgeRetirementTestConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *sessionEdgeRetirementTestConn) Write(data []byte) (int, error) { return len(data), nil }
func (c *sessionEdgeRetirementTestConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}
func (c *sessionEdgeRetirementTestConn) LocalAddr() net.Addr              { return tcpAddr("127.0.0.1", 39444) }
func (c *sessionEdgeRetirementTestConn) RemoteAddr() net.Addr             { return c.remote }
func (c *sessionEdgeRetirementTestConn) SetDeadline(time.Time) error      { return nil }
func (c *sessionEdgeRetirementTestConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sessionEdgeRetirementTestConn) SetWriteDeadline(time.Time) error { return nil }
func (c *sessionEdgeRetirementTestConn) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
