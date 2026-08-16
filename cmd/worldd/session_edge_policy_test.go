package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSessionEdgePolicyLoadsNetworkIdentityBindings(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca-a", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{"EDGE-A.Astrahold.Test"}},
			{Prefixes: []string{"10.0.0.0/8"}, DNSNames: []string{"internal-hop.astrahold.test"}},
		},
	})

	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 1 || metadata.Revision != "edge-001" || metadata.HeaderMode != "x-forwarded-for" || metadata.RootCount != 1 || metadata.BindingCount != 2 || metadata.PrefixCount != 2 || metadata.IdentityCount != 2 {
		t.Fatalf("metadata=%+v", metadata)
	}
	snapshot, _ := runtime.currentSnapshot()
	if binding, ok := snapshot.bindingForPeer(mustSessionEdgeAddr(t, "127.0.0.2")); !ok || binding != 0 {
		t.Fatalf("binding=%d ok=%v", binding, ok)
	}
	if !snapshot.trusted(mustSessionEdgeAddr(t, "10.4.5.6")) {
		t.Fatal("trusted intermediary prefix was not included in the generation")
	}
}

func TestSessionEdgePolicyRejectsOverlappingBindings(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-overlap",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"10.0.0.0/8"}, DNSNames: []string{"edge-a.astrahold.test"}},
			{Prefixes: []string{"10.1.0.0/16"}, DNSNames: []string{"edge-b.astrahold.test"}},
		},
	})
	if _, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now }); err == nil {
		t.Fatal("overlapping network bindings unexpectedly accepted")
	}
}

func TestSessionEdgePolicyTLSSelectionRequiresBoundExactIdentity(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	if config.GetConfigForClient == nil {
		t.Fatal("edge policy did not install TLS selector")
	}

	directConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.1", 5000)}})
	if err != nil {
		t.Fatal(err)
	}
	if directConfig != nil {
		t.Fatal("direct peer unexpectedly required a proxy client certificate")
	}

	proxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 5001)}})
	if err != nil {
		t.Fatal(err)
	}
	if proxyConfig == nil || proxyConfig.ClientAuth != tls.RequireAndVerifyClientCert || proxyConfig.VerifyConnection == nil {
		t.Fatalf("proxy config=%+v", proxyConfig)
	}
	wrongLeaf := &x509.Certificate{DNSNames: []string{"edge-b.astrahold.test"}}
	wrongState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{wrongLeaf}, VerifiedChains: [][]*x509.Certificate{{wrongLeaf}}}
	if err := proxyConfig.VerifyConnection(wrongState); err == nil {
		t.Fatal("cross-binding proxy identity unexpectedly accepted")
	}
	allowedLeaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}}
	allowedState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{allowedLeaf}, VerifiedChains: [][]*x509.Certificate{{allowedLeaf}}}
	if err := proxyConfig.VerifyConnection(allowedState); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:5001"
	request.Header.Set("X-Forwarded-For", "198.51.100.20, 10.0.0.9")
	request.TLS = &allowedState
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "198.51.100.20" {
		t.Fatalf("source=%q", source)
	}
}

func TestSessionEdgePolicyReloadPinsEstablishedConnectionGeneration(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
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
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}

	oldHello := &tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 5100)}}
	oldConfig, err := config.GetConfigForClient(oldHello)
	if err != nil {
		t.Fatal(err)
	}
	oldLeaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}}
	oldState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{oldLeaf}, VerifiedChains: [][]*x509.Certificate{{oldLeaf}}}
	if err := oldConfig.VerifyConnection(oldState); err != nil {
		t.Fatal(err)
	}

	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-002",
		ForwardedHeader: "forwarded",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: []string{"edge-a-v2.astrahold.test"},
		}},
	})
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousGeneration != 1 || result.Generation != 2 || result.PreviousHeaderMode != "x-forwarded-for" || result.HeaderMode != "forwarded" {
		t.Fatalf("reload=%+v", result)
	}

	oldRequest := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	oldRequest.RemoteAddr = "127.0.0.2:5100"
	oldRequest.Header.Set("X-Forwarded-For", "198.51.100.30")
	oldRequest.TLS = &oldState
	oldSource, err := attributor.sourceIP(oldRequest)
	if err != nil || oldSource != "198.51.100.30" {
		t.Fatalf("old connection source=%q err=%v", oldSource, err)
	}

	newConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 5101)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := newConfig.VerifyConnection(oldState); err == nil {
		t.Fatal("old identity unexpectedly accepted for a new generation-2 handshake")
	}
	newLeaf := &x509.Certificate{DNSNames: []string{"edge-a-v2.astrahold.test"}}
	newState := tls.ConnectionState{PeerCertificates: []*x509.Certificate{newLeaf}, VerifiedChains: [][]*x509.Certificate{{newLeaf}}}
	if err := newConfig.VerifyConnection(newState); err != nil {
		t.Fatal(err)
	}
	newRequest := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	newRequest.RemoteAddr = "127.0.0.2:5101"
	newRequest.Header.Set("Forwarded", "for=198.51.100.31")
	newRequest.TLS = &newState
	newSource, err := attributor.sourceIP(newRequest)
	if err != nil || newSource != "198.51.100.31" {
		t.Fatalf("new connection source=%q err=%v", newSource, err)
	}

	attributor.releaseConnection(oldRequest.RemoteAddr)
	if _, err := attributor.sourceIP(oldRequest); err == nil {
		t.Fatal("released connection from a currently trusted peer did not fail closed")
	}
}

func TestSessionEdgePolicyReloadRetainsLKGOnInvalidReplacement(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{"edge-a.astrahold.test"}}},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "bad",
		ForwardedHeader: "forwarded",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"127.0.0.0/8"}, DNSNames: []string{"edge-a.astrahold.test"}},
			{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{"edge-b.astrahold.test"}},
		},
	})
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("invalid overlapping replacement unexpectedly accepted")
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 1 || metadata.Revision != "edge-001" || metadata.HeaderMode != "x-forwarded-for" {
		t.Fatalf("LKG changed: %+v", metadata)
	}
}

func TestSessionEdgePolicyConcurrentSnapshotAndReload(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	runtime := newSessionEdgePolicyTestRuntime(t, now, "edge-001", "x-forwarded-for", "edge-a.astrahold.test")
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 100; iteration++ {
				metadata := runtime.Snapshot()
				if metadata.Generation == 0 || metadata.Revision == "" || metadata.BindingCount != 2 {
					t.Errorf("metadata=%+v", metadata)
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

func newSessionEdgePolicyTestRuntime(t *testing.T, now time.Time, revision, header, identity string) *reloadableSessionEdgePolicy {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        revision,
		ForwardedHeader: header,
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{identity}},
			{Prefixes: []string{"10.0.0.0/8"}, DNSNames: []string{"internal-hop.astrahold.test"}},
		},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func writeSessionEdgePolicyTest(t *testing.T, path string, definition sessionEdgePolicyDefinition) {
	t.Helper()
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustSessionEdgeAddr(t *testing.T, value string) netip.Addr {
	t.Helper()
	address, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatal(err)
	}
	return address
}
