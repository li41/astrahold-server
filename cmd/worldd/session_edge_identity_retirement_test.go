package main

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSessionEdgeIdentityHandshakePinsOnlyOriginallyAllowedMatches(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	runtime := newSessionEdgeIdentityRetirementTestRuntime(t, now, "edge-001", []string{
		"edge-a.astrahold.test",
		"edge-canary.astrahold.test",
	})
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	remote := "127.0.0.2:6700"
	proxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6700)}})
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{DNSNames: []string{
		"EDGE-A.ASTRAHOLD.TEST",
		"edge-future.astrahold.test",
		"edge-a.astrahold.test",
	}}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	if err := proxyConfig.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
	binding, ok := runtime.connection(remote)
	if !ok {
		t.Fatal("verified proxy connection was not bound")
	}
	want := []string{"edge-a.astrahold.test"}
	if !reflect.DeepEqual(binding.matchedDNS, want) {
		t.Fatalf("matched identities=%v want=%v", binding.matchedDNS, want)
	}

	writeSessionEdgeIdentityRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", []string{
		"edge-future.astrahold.test",
		"edge-next.astrahold.test",
	})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation != 2 || !result.AuthorityChanged {
		t.Fatalf("reload=%+v", result)
	}
	if !runtime.connectionRequiresRetirement(remote, binding) {
		t.Fatal("old multi-SAN connection was retroactively promoted by a newly allowed SAN")
	}
}

func TestSessionEdgeIdentityRetirementPreservesStillAllowedIdentityWithinBinding(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	runtime := newSessionEdgeIdentityRetirementTestRuntime(t, now, "edge-001", []string{
		"edge-a.astrahold.test",
		"edge-canary.astrahold.test",
	})
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	a := newSessionEdgeRetirementTestConn("127.0.0.2", 6710)
	canary := newSessionEdgeRetirementTestConn("127.0.0.2", 6711)
	bindSessionEdgeIdentityRetirementTestConnection(t, attributor, runtime, a, []string{"edge-a.astrahold.test"})
	bindSessionEdgeIdentityRetirementTestConnection(t, attributor, runtime, canary, []string{"edge-canary.astrahold.test"})

	writeSessionEdgeIdentityRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", []string{
		"edge-a.astrahold.test",
		"edge-next.astrahold.test",
	})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if a.Closed() {
		t.Fatal("connection authenticated as still-allowed identity was retired")
	}
	if !canary.Closed() {
		t.Fatal("connection authenticated only as removed identity survived")
	}
}

func TestSessionEdgeIdentityRetirementUsesIntersectionOfOriginallyMatchedMultiSANs(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	runtime := newSessionEdgeIdentityRetirementTestRuntime(t, now, "edge-001", []string{
		"edge-a.astrahold.test",
		"edge-b.astrahold.test",
	})
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	connection := newSessionEdgeRetirementTestConn("127.0.0.2", 6720)
	bindSessionEdgeIdentityRetirementTestConnection(t, attributor, runtime, connection, []string{
		"edge-b.astrahold.test",
		"edge-a.astrahold.test",
	})

	writeSessionEdgeIdentityRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", []string{
		"edge-b.astrahold.test",
		"edge-next.astrahold.test",
	})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retired := attributor.retireOldEdgeConnections(result.Generation); retired != 0 {
		t.Fatalf("retired=%d want=0", retired)
	}
	if connection.Closed() {
		t.Fatal("connection lost all authority even though one originally matched SAN remains allowed")
	}
}

func TestSessionEdgeIdentityRetirementLateHandshakeUsesPinnedMatchedIdentity(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	runtime := newSessionEdgeIdentityRetirementTestRuntime(t, now, "edge-001", []string{
		"edge-a.astrahold.test",
		"edge-canary.astrahold.test",
	})
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	oldSnapshot, oldGeneration := runtime.currentSnapshot()
	bindingIndex, ok := oldSnapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.2"))
	if !ok {
		t.Fatal("old snapshot missing trusted binding")
	}

	writeSessionEdgeIdentityRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", []string{
		"edge-a.astrahold.test",
		"edge-next.astrahold.test",
	})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation <= oldGeneration {
		t.Fatalf("reload generation=%d old=%d", result.Generation, oldGeneration)
	}

	stillAllowed := newSessionEdgeRetirementTestConn("127.0.0.2", 6730)
	attributor.observeEdgeConnectionState(stillAllowed, http.StateNew)
	runtime.bindVerifiedConnection(stillAllowed.RemoteAddr().String(), oldSnapshot, oldGeneration, bindingIndex, []string{"edge-a.astrahold.test"})
	attributor.observeEdgeConnectionState(stillAllowed, http.StateActive)
	if stillAllowed.Closed() {
		t.Fatal("late handshake using still-allowed pinned identity was retired")
	}

	removed := newSessionEdgeRetirementTestConn("127.0.0.2", 6731)
	attributor.observeEdgeConnectionState(removed, http.StateNew)
	runtime.bindVerifiedConnection(removed.RemoteAddr().String(), oldSnapshot, oldGeneration, bindingIndex, []string{"edge-canary.astrahold.test"})
	attributor.observeEdgeConnectionState(removed, http.StateActive)
	if !removed.Closed() {
		t.Fatal("late handshake using removed pinned identity survived")
	}
}

func TestSessionEdgeIdentityRetirementFailsClosedWithoutPinnedIdentity(t *testing.T) {
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	runtime := newSessionEdgeIdentityRetirementTestRuntime(t, now, "edge-001", []string{"edge-a.astrahold.test"})
	oldSnapshot, oldGeneration := runtime.currentSnapshot()
	writeSessionEdgeIdentityRetirementPolicy(t, runtime, "edge-002", "x-forwarded-for", []string{
		"edge-a.astrahold.test",
		"edge-next.astrahold.test",
	})
	if _, err := runtime.Reload(); err != nil {
		t.Fatal(err)
	}
	bindingIndex, ok := oldSnapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.2"))
	if !ok {
		t.Fatal("old snapshot missing trusted binding")
	}
	binding := sessionEdgePolicyConnection{snapshot: oldSnapshot, generation: oldGeneration, bindingIndex: bindingIndex}
	if !runtime.connectionRequiresRetirement("127.0.0.2:6740", binding) {
		t.Fatal("old connection without pinned authenticated identity did not fail closed")
	}
}

func newSessionEdgeIdentityRetirementTestRuntime(t *testing.T, now time.Time, revision string, dnsNames []string) *reloadableSessionEdgePolicy {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        revision,
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: append([]string(nil), dnsNames...),
		}},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func writeSessionEdgeIdentityRetirementPolicy(t *testing.T, runtime *reloadableSessionEdgePolicy, revision, header string, dnsNames []string) {
	t.Helper()
	if runtime == nil {
		t.Fatal("nil edge policy runtime")
	}
	writeSessionEdgePolicyTest(t, runtime.definitionFile, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        revision,
		ForwardedHeader: header,
		ClientCAFile:    "edge-ca.pem",
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: append([]string(nil), dnsNames...),
		}},
	})
}

func bindSessionEdgeIdentityRetirementTestConnection(t *testing.T, attributor *sessionSourceAttributor, runtime *reloadableSessionEdgePolicy, connection *sessionEdgeRetirementTestConn, matchedDNS []string) {
	t.Helper()
	if attributor == nil || runtime == nil || connection == nil {
		t.Fatal("invalid identity retirement test fixture")
	}
	attributor.observeEdgeConnectionState(connection, http.StateNew)
	snapshot, generation := runtime.currentSnapshot()
	peer := sessionLoginSourceIP(connection.RemoteAddr().String())
	bindingIndex, ok := snapshot.bindingForPeer(mustSessionEdgeAddr(t, peer))
	if !ok {
		t.Fatalf("snapshot has no binding for %s", peer)
	}
	runtime.bindVerifiedConnection(connection.RemoteAddr().String(), snapshot, generation, bindingIndex, matchedDNS)
	attributor.observeEdgeConnectionState(connection, http.StateActive)
}
