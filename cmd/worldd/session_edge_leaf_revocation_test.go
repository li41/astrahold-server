package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSessionLeafRevocationStrictParsingDedupAndNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revocations.json")
	a := sha256.Sum256([]byte("leaf-a-spki"))
	writeSessionLeafRevocationTest(t, path, "rev-001", []string{hex.EncodeToString(a[:]), hex.EncodeToString(a[:])})
	runtime, err := newReloadableSessionLeafRevocation(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 1 || metadata.Revision != "rev-001" || metadata.RevokedCredentialCount != 1 {
		t.Fatalf("metadata=%+v", metadata)
	}

	writeSessionLeafRevocationTest(t, path, "representation-only", []string{hex.EncodeToString(a[:]), hex.EncodeToString(a[:])})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthorityChanged || result.Generation != 1 || result.Revision != "rev-001" {
		t.Fatalf("no-op reload=%+v", result)
	}

	bad := []byte(`{"schema_version":1,"revision":"bad","revoked_spki_sha256":[],"unknown":true}`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("unknown field unexpectedly accepted")
	}
	if got := runtime.Snapshot(); got.Generation != 1 || got.Revision != "rev-001" || got.RevokedCredentialCount != 1 {
		t.Fatalf("LKG changed after invalid reload: %+v", got)
	}

	writeSessionLeafRevocationTest(t, path, "bad-case", []string{hex.EncodeToString(a[:])[:63] + "A"})
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("non-lowercase fingerprint unexpectedly accepted")
	}
}

func TestSessionLeafRevocationSameIdentityCredentialFence(t *testing.T) {
	setSessionEdgeRetirementForTest(t, true)
	now := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	runtime, revocationPath := newSessionLeafRevocationEdgeRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}

	aSPKI := []byte("leaf-a-spki-instance")
	bSPKI := []byte("leaf-b-spki-instance")
	aID := sha256.Sum256(aSPKI)
	bID := sha256.Sum256(bSPKI)
	aConn := newSessionEdgeRetirementTestConn("127.0.0.2", 6810)
	bConn := newSessionEdgeRetirementTestConn("127.0.0.2", 6811)
	bindSessionLeafRevocationTestConnection(t, attributor, runtime, aConn, aID)
	bindSessionLeafRevocationTestConnection(t, attributor, runtime, bConn, bID)

	writeSessionLeafRevocationTest(t, revocationPath, "rev-002", []string{hex.EncodeToString(aID[:])})
	result, err := runtime.ReloadLeafRevocation()
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation != 2 || !result.AuthorityChanged || result.RevokedCredentialCount != 1 {
		t.Fatalf("reload=%+v", result)
	}
	if retired := attributor.retireOldEdgeConnections(runtime.Snapshot().Generation); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if !aConn.Closed() {
		t.Fatal("revoked Leaf A established connection survived")
	}
	if bConn.Closed() {
		t.Fatal("healthy Leaf B with the same DNS identity was retired")
	}

	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	aProxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6812)}})
	if err != nil {
		t.Fatal(err)
	}
	aLeaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}, RawSubjectPublicKeyInfo: aSPKI}
	aState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{aLeaf}, VerifiedChains: [][]*x509.Certificate{{aLeaf}}}
	if err := aProxyConfig.VerifyConnection(aState); err == nil {
		t.Fatal("fresh revoked Leaf A handshake was accepted")
	}

	bProxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6813)}})
	if err != nil {
		t.Fatal(err)
	}
	bLeaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}, RawSubjectPublicKeyInfo: bSPKI}
	bState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{bLeaf}, VerifiedChains: [][]*x509.Certificate{{bLeaf}}}
	if err := bProxyConfig.VerifyConnection(bState); err != nil {
		t.Fatal(err)
	}

	directConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.1", 6814)}})
	if err != nil {
		t.Fatal(err)
	}
	if directConfig != nil {
		t.Fatal("direct client unexpectedly gained proxy mTLS requirements")
	}
}

func TestSessionLeafRevocationLateHandshakeFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	runtime, revocationPath := newSessionLeafRevocationEdgeRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	proxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6820)}})
	if err != nil {
		t.Fatal(err)
	}
	spki := []byte("late-leaf-a-spki")
	id := sha256.Sum256(spki)
	writeSessionLeafRevocationTest(t, revocationPath, "rev-002", []string{hex.EncodeToString(id[:])})
	if _, err := runtime.ReloadLeafRevocation(); err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}, RawSubjectPublicKeyInfo: spki}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	if err := proxyConfig.VerifyConnection(state); err == nil {
		t.Fatal("late handshake captured before revocation publication escaped the current revocation fence")
	}
}

func TestSessionLeafRevocationRequestPathFailsClosedBeforeSocketRetirement(t *testing.T) {
	now := time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	runtime, revocationPath := newSessionLeafRevocationEdgeRuntime(t, now)
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	spki := []byte("request-race-spki")
	id := sha256.Sum256(spki)
	snapshot, generation := runtime.currentSnapshot()
	bindingIndex, ok := snapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.2"))
	if !ok {
		t.Fatal("missing test binding")
	}
	if err := runtime.bindVerifiedConnectionCredential("127.0.0.2:6830", snapshot, generation, bindingIndex, []string{"edge-a.astrahold.test"}, id, true); err != nil {
		t.Fatal(err)
	}
	writeSessionLeafRevocationTest(t, revocationPath, "rev-002", []string{hex.EncodeToString(id[:])})
	if _, err := runtime.ReloadLeafRevocation(); err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}, RawSubjectPublicKeyInfo: spki}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:6830"
	request.Header.Set("X-Forwarded-For", "198.51.100.200")
	request.TLS = &state
	if _, err := attributor.sourceIP(request); err == nil {
		t.Fatal("revoked established connection retained forwarding authority before socket close")
	}
}

func TestSessionLeafRevocationConcurrentReloadAndLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revocations.json")
	writeSessionLeafRevocationTest(t, path, "rev-001", nil)
	runtime, err := newReloadableSessionLeafRevocation(path)
	if err != nil {
		t.Fatal(err)
	}
	id := sha256.Sum256([]byte("concurrent-spki"))
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = runtime.revokedCredential(id)
				_ = runtime.Snapshot()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		values := []string(nil)
		if i%2 == 0 {
			values = []string{hex.EncodeToString(id[:])}
		}
		writeSessionLeafRevocationTest(t, path, "rev-concurrent", values)
		if _, err := runtime.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func newSessionLeafRevocationEdgeRuntime(t *testing.T, now time.Time) (*reloadableSessionEdgePolicy, string) {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	revocationPath := filepath.Join(dir, "leaf-revocations.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: []string{"edge-a.astrahold.test"},
		}},
	})
	writeSessionLeafRevocationTest(t, revocationPath, "rev-001", nil)
	previous := *sessionLoginTrustedProxyLeafRevocationFile
	*sessionLoginTrustedProxyLeafRevocationFile = revocationPath
	t.Cleanup(func() { *sessionLoginTrustedProxyLeafRevocationFile = previous })
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return runtime, revocationPath
}

func bindSessionLeafRevocationTestConnection(t *testing.T, attributor *sessionSourceAttributor, runtime *reloadableSessionEdgePolicy, connection *sessionEdgeRetirementTestConn, credentialID [sha256.Size]byte) {
	t.Helper()
	attributor.observeEdgeConnectionState(connection, http.StateNew)
	snapshot, generation := runtime.currentSnapshot()
	peer := sessionLoginSourceIP(connection.RemoteAddr().String())
	bindingIndex, ok := snapshot.bindingForPeer(mustSessionEdgeAddr(t, peer))
	if !ok {
		t.Fatalf("snapshot has no binding for %s", peer)
	}
	if err := runtime.bindVerifiedConnectionCredential(connection.RemoteAddr().String(), snapshot, generation, bindingIndex, []string{"edge-a.astrahold.test"}, credentialID, true); err != nil {
		t.Fatal(err)
	}
	attributor.observeEdgeConnectionState(connection, http.StateActive)
}

func writeSessionLeafRevocationTest(t *testing.T, path, revision string, revoked []string) {
	t.Helper()
	data, err := json.Marshal(sessionLeafRevocationDefinition{
		SchemaVersion:     sessionLeafRevocationSchemaVersion,
		Revision:          revision,
		RevokedSPKISHA256: append([]string(nil), revoked...),
	})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
