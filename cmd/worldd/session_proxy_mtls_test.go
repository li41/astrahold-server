package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSessionProxyMTLSLoadsBoundedPolicyAndExactIdentity(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "proxy-ca.pem")
	writeSessionProxyTestCA(t, caPath, 1, "proxy-ca-a", now)
	policyPath := filepath.Join(dir, "proxy-policy.json")
	writeSessionProxyTestPolicy(t, policyPath, "proxy-001", filepath.Base(caPath), []string{"edge-a.astrahold.test", "EDGE-B.astrahold.test"})

	runtime, err := newReloadableSessionProxyMTLS(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 1 || metadata.Revision != "proxy-001" || metadata.RootCount != 1 || metadata.IdentityCount != 2 {
		t.Fatalf("metadata=%+v", metadata)
	}

	snapshot, _ := runtime.currentSnapshot()
	allowedLeaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}}
	allowedState := tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{allowedLeaf},
		VerifiedChains:   [][]*x509.Certificate{{allowedLeaf}},
	}
	if err := snapshot.verifyConnection(allowedState); err != nil {
		t.Fatalf("allowed identity rejected: %v", err)
	}
	wrongLeaf := &x509.Certificate{DNSNames: []string{"other.astrahold.test"}}
	wrongState := tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{wrongLeaf},
		VerifiedChains:   [][]*x509.Certificate{{wrongLeaf}},
	}
	if err := snapshot.verifyConnection(wrongState); err == nil {
		t.Fatal("unexpectedly accepted non-allowlisted DNS identity")
	}
	wildcardLeaf := &x509.Certificate{DNSNames: []string{"*.astrahold.test"}}
	wildcardState := tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{wildcardLeaf},
		VerifiedChains:   [][]*x509.Certificate{{wildcardLeaf}},
	}
	if err := snapshot.verifyConnection(wildcardState); err == nil {
		t.Fatal("unexpectedly accepted wildcard identity")
	}
}

func TestSessionProxyMTLSReloadPublishesGenerationAndRetainsLKG(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "proxy-ca.pem")
	policyPath := filepath.Join(dir, "proxy-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "proxy-ca-a", now)
	writeSessionProxyTestPolicy(t, policyPath, "proxy-001", filepath.Base(caPath), []string{"edge.astrahold.test"})

	runtime, err := newReloadableSessionProxyMTLS(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	writeSessionProxyTestCA(t, caPath, 2, "proxy-ca-b", now)
	writeSessionProxyTestPolicy(t, policyPath, "proxy-002", filepath.Base(caPath), []string{"edge.astrahold.test", "edge-b.astrahold.test"})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousGeneration != 1 || result.Generation != 2 || result.PreviousRevision != "proxy-001" || result.Revision != "proxy-002" || result.IdentityCount != 2 {
		t.Fatalf("reload result=%+v", result)
	}

	if err := os.WriteFile(caPath, []byte("not a PEM bundle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("invalid replacement unexpectedly accepted")
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 2 || metadata.Revision != "proxy-002" || metadata.IdentityCount != 2 {
		t.Fatalf("last-known-good changed after rejected reload: %+v", metadata)
	}
}

func TestSessionProxyMTLSTLSSelectionOnlyRequiresClientCertificateForTrustedPeer(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "proxy-ca.pem")
	policyPath := filepath.Join(dir, "proxy-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "proxy-ca", now)
	writeSessionProxyTestPolicy(t, policyPath, "proxy-001", filepath.Base(caPath), []string{"edge.astrahold.test"})
	proxyMTLS, err := newReloadableSessionProxyMTLS(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	attributor.proxyMTLS = proxyMTLS
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	if config.GetConfigForClient == nil {
		t.Fatal("mTLS mode did not install per-peer TLS selector")
	}

	directConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.1", 5000)}})
	if err != nil {
		t.Fatal(err)
	}
	if directConfig != nil {
		t.Fatal("direct/untrusted peer unexpectedly received client-auth TLS config")
	}
	proxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 5000)}})
	if err != nil {
		t.Fatal(err)
	}
	if proxyConfig == nil || proxyConfig.ClientAuth != tls.RequireAndVerifyClientCert || proxyConfig.ClientCAs == nil || proxyConfig.VerifyConnection == nil {
		t.Fatalf("trusted proxy TLS config=%+v", proxyConfig)
	}
}

func TestSessionSourceAttributorMTLSRequiresVerifiedTLSStateBeforeForwarding(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "proxy-ca.pem")
	policyPath := filepath.Join(dir, "proxy-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "proxy-ca", now)
	writeSessionProxyTestPolicy(t, policyPath, "proxy-001", filepath.Base(caPath), []string{"edge.astrahold.test"})
	proxyMTLS, err := newReloadableSessionProxyMTLS(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	attributor.proxyMTLS = proxyMTLS

	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:5000"
	request.Header.Set("X-Forwarded-For", "198.51.100.90")
	if _, err := attributor.sourceIP(request); err == nil {
		t.Fatal("trusted proxy forwarding accepted without verified mTLS state")
	}
	leaf := &x509.Certificate{DNSNames: []string{"edge.astrahold.test"}}
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains:   [][]*x509.Certificate{{leaf}},
	}
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "198.51.100.90" {
		t.Fatalf("source=%q", source)
	}
}

func TestSessionProxyMTLSConcurrentSnapshotAndReload(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "proxy-ca.pem")
	policyPath := filepath.Join(dir, "proxy-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "proxy-ca", now)
	writeSessionProxyTestPolicy(t, policyPath, "proxy-001", filepath.Base(caPath), []string{"edge.astrahold.test"})
	runtime, err := newReloadableSessionProxyMTLS(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 100; iteration++ {
				metadata := runtime.Snapshot()
				if metadata.Generation == 0 || metadata.Revision == "" {
					t.Errorf("invalid snapshot: %+v", metadata)
					return
				}
			}
		}()
	}
	for iteration := 0; iteration < 8; iteration++ {
		if _, err := runtime.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func TestNormalizeSessionProxyDNSIdentityRejectsWildcardIPAndMalformedNames(t *testing.T) {
	for _, value := range []string{"*.astrahold.test", "127.0.0.1", "bad_name.astrahold.test", "-edge.astrahold.test", "edge..astrahold.test", "edge.astrahold.test."} {
		if _, err := normalizeSessionProxyDNSIdentity(value); err == nil {
			t.Fatalf("identity %q unexpectedly accepted", value)
		}
	}
	if normalized, err := normalizeSessionProxyDNSIdentity("EDGE-1.Astrahold.Test"); err != nil || normalized != "edge-1.astrahold.test" {
		t.Fatalf("normalized=%q err=%v", normalized, err)
	}
}

func writeSessionProxyTestCA(t *testing.T, path string, serial int64, commonName string, now time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSessionProxyTestPolicy(t *testing.T, path, revision, caFile string, dnsNames []string) {
	t.Helper()
	data, err := json.Marshal(sessionProxyMTLSDefinition{
		SchemaVersion: sessionProxyMTLSSchemaVersion,
		Revision:      revision,
		ClientCAFile:  caFile,
		DNSNames:      dnsNames,
	})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

type sessionProxyTestConn struct {
	remote net.Addr
}

func (c *sessionProxyTestConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *sessionProxyTestConn) Write(data []byte) (int, error) { return len(data), nil }
func (c *sessionProxyTestConn) Close() error                    { return nil }
func (c *sessionProxyTestConn) LocalAddr() net.Addr             { return tcpAddr("127.0.0.1", 39444) }
func (c *sessionProxyTestConn) RemoteAddr() net.Addr            { return c.remote }
func (c *sessionProxyTestConn) SetDeadline(time.Time) error     { return nil }
func (c *sessionProxyTestConn) SetReadDeadline(time.Time) error { return nil }
func (c *sessionProxyTestConn) SetWriteDeadline(time.Time) error { return nil }

func tcpAddr(ip string, port int) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}
