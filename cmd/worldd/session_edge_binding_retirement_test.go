package main

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionEdgeBindingRetirementKeepsUnaffectedBinding(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC)
	runtime := newSessionEdgeBindingRetirementTestRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	a := newSessionEdgeRetirementTestConn("127.0.0.2", 6600)
	b := newSessionEdgeRetirementTestConn("127.0.0.3", 6601)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, a)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, b)

	writeSessionEdgeBindingRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", "edge-a2.astrahold.test", "edge-b.astrahold.test", "127.0.0.3/32")
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if !result.AuthorityChanged || result.Generation != 2 {
		t.Fatalf("reload=%+v", result)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if !a.Closed() {
		t.Fatal("changed binding connection survived identity cutover")
	}
	if b.Closed() {
		t.Fatal("unchanged binding connection was retired")
	}
}

func TestSessionEdgeBindingRetirementLateHandshakeUsesPeerCompatibility(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC)
	runtime := newSessionEdgeBindingRetirementTestRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	oldSnapshot, oldGeneration := runtime.currentSnapshot()

	writeSessionEdgeBindingRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", "edge-a2.astrahold.test", "edge-b.astrahold.test", "127.0.0.3/32")
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation <= oldGeneration {
		t.Fatalf("reload generation=%d old=%d", result.Generation, oldGeneration)
	}

	unchanged := newSessionEdgeRetirementTestConn("127.0.0.3", 6610)
	attributor.observeEdgeConnectionState(unchanged, http.StateNew)
	unchangedBinding, ok := oldSnapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.3"))
	if !ok {
		t.Fatal("old snapshot missing unchanged binding")
	}
	runtime.bindConnection(unchanged.RemoteAddr().String(), oldSnapshot, oldGeneration, unchangedBinding)
	attributor.observeEdgeConnectionState(unchanged, http.StateActive)
	if unchanged.Closed() {
		t.Fatal("late handshake for unchanged binding was retired")
	}

	changed := newSessionEdgeRetirementTestConn("127.0.0.2", 6611)
	attributor.observeEdgeConnectionState(changed, http.StateNew)
	changedBinding, ok := oldSnapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.2"))
	if !ok {
		t.Fatal("old snapshot missing changed binding")
	}
	runtime.bindConnection(changed.RemoteAddr().String(), oldSnapshot, oldGeneration, changedBinding)
	attributor.observeEdgeConnectionState(changed, http.StateActive)
	if !changed.Closed() {
		t.Fatal("late handshake for changed binding survived")
	}
}

func TestSessionEdgeBindingRetirementHeaderChangeRemainsGlobal(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC)
	runtime := newSessionEdgeBindingRetirementTestRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	a := newSessionEdgeRetirementTestConn("127.0.0.2", 6620)
	b := newSessionEdgeRetirementTestConn("127.0.0.3", 6621)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, a)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, b)

	writeSessionEdgeBindingRetirementPolicy(t, runtime, "edge-002", "forwarded", "edge-a.astrahold.test", "edge-b.astrahold.test", "127.0.0.3/32")
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 2 {
		t.Fatalf("retired=%d want=2", retired)
	}
	if !a.Closed() || !b.Closed() {
		t.Fatalf("global header cutover left connections open: a=%v b=%v", a.Closed(), b.Closed())
	}
}

func TestSessionEdgeBindingRetirementPrefixTopologyChangeRemainsGlobal(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC)
	runtime := newSessionEdgeBindingRetirementTestRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	a := newSessionEdgeRetirementTestConn("127.0.0.2", 6630)
	b := newSessionEdgeRetirementTestConn("127.0.0.3", 6631)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, a)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, b)

	writeSessionEdgeBindingRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", "edge-a.astrahold.test", "edge-b.astrahold.test", "127.0.0.4/32")
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 2 {
		t.Fatalf("retired=%d want=2", retired)
	}
	if !a.Closed() || !b.Closed() {
		t.Fatalf("global topology cutover left connections open: a=%v b=%v", a.Closed(), b.Closed())
	}
}

func TestSessionEdgeBindingRetirementCARootChangeRemainsGlobal(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC)
	runtime := newSessionEdgeBindingRetirementTestRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	a := newSessionEdgeRetirementTestConn("127.0.0.2", 6640)
	b := newSessionEdgeRetirementTestConn("127.0.0.3", 6641)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, a)
	bindSessionEdgeRetirementTestConnection(t, attributor, runtime, b)

	caPath := filepath.Join(filepath.Dir(runtime.definitionFile), "edge-ca.pem")
	writeSessionProxyTestCA(t, caPath, 2, "edge-ca-rotated", now)
	writeSessionEdgeBindingRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", "edge-a.astrahold.test", "edge-b.astrahold.test", "127.0.0.3/32")
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 2 {
		t.Fatalf("retired=%d want=2", retired)
	}
	if !a.Closed() || !b.Closed() {
		t.Fatalf("global CA cutover left connections open: a=%v b=%v", a.Closed(), b.Closed())
	}
}

func newSessionEdgeBindingRetirementTestRuntime(t *testing.T, now time.Time) *reloadableSessionEdgePolicy {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{"edge-a.astrahold.test"}},
			{Prefixes: []string{"127.0.0.3/32"}, DNSNames: []string{"edge-b.astrahold.test"}},
		},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func writeSessionEdgeBindingRetirementPolicy(t *testing.T, runtime *reloadableSessionEdgePolicy, revision, header, identityA, identityB, prefixB string) {
	t.Helper()
	if runtime == nil {
		t.Fatal("nil edge policy runtime")
	}
	writeSessionEdgePolicyTest(t, runtime.definitionFile, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        revision,
		ForwardedHeader: header,
		ClientCAFile:    "edge-ca.pem",
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{identityA}},
			{Prefixes: []string{prefixB}, DNSNames: []string{identityB}},
		},
	})
}

func bindSessionEdgeRetirementTestConnection(t *testing.T, attributor *sessionSourceAttributor, runtime *reloadableSessionEdgePolicy, connection *sessionEdgeRetirementTestConn) {
	t.Helper()
	if attributor == nil || runtime == nil || connection == nil {
		t.Fatal("invalid retirement test fixture")
	}
	attributor.observeEdgeConnectionState(connection, http.StateNew)
	snapshot, generation := runtime.currentSnapshot()
	peer := sessionLoginSourceIP(connection.RemoteAddr().String())
	bindingIndex, ok := snapshot.bindingForPeer(mustSessionEdgeAddr(t, peer))
	if !ok {
		t.Fatalf("snapshot has no binding for %s", peer)
	}
	runtime.bindConnection(connection.RemoteAddr().String(), snapshot, generation, bindingIndex)
	attributor.observeEdgeConnectionState(connection, http.StateActive)
}
