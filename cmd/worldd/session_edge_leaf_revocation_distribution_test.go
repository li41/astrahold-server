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
	"testing"
	"time"
)

func TestSessionLeafRevocationDistributionEpochRenewalRollbackAndDurableAck(t *testing.T) {
	now := time.Date(2026, 8, 16, 16, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	revocationPath := filepath.Join(dir, "revocations.json")
	distributionPath := filepath.Join(dir, "distribution.json")
	ackPath := filepath.Join(dir, "acks", "instance-a.json")
	writeSessionLeafRevocationTest(t, revocationPath, "rev-001", nil)
	digest := sessionLeafRevocationAuthorityDigest(map[[sha256.Size]byte]struct{}{})
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 1, digest, now.Add(10*time.Minute))
	setSessionLeafRevocationDistributionForTest(t, distributionPath, "instance-a", ackPath)

	runtime, err := newReloadableSessionLeafRevocationWithClock(revocationPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	metadata := runtime.distribution.metadata(now)
	if !metadata.Enabled || metadata.Epoch != 1 || !metadata.AckHealthy || !metadata.LeaseValid {
		t.Fatalf("distribution metadata=%+v", metadata)
	}
	ack, err := loadSessionLeafRevocationDistributionAck(ackPath)
	if err != nil {
		t.Fatal(err)
	}
	if ack.InstanceID != "instance-a" || ack.Epoch != 1 || ack.RevocationAuthoritySHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("ack=%+v", ack)
	}

	writeSessionLeafRevocationDistributionTest(t, distributionPath, 2, digest, now.Add(20*time.Minute))
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthorityChanged || !result.DistributionChanged || result.Generation != 1 || result.DistributionEpoch != 2 || !result.DistributionAckHealthy {
		t.Fatalf("renewal=%+v", result)
	}

	writeSessionLeafRevocationDistributionTest(t, distributionPath, 1, digest, now.Add(30*time.Minute))
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("distribution rollback unexpectedly accepted")
	}
	if got := runtime.distribution.metadata(now); got.Epoch != 2 || !got.AckHealthy {
		t.Fatalf("LKG distribution changed after rollback: %+v", got)
	}

	if _, err := newReloadableSessionLeafRevocationWithClock(revocationPath, func() time.Time { return now }); err == nil {
		t.Fatal("restart accepted distribution epoch below durable ack floor")
	}
}

func TestSessionLeafRevocationDistributionBindsManifestToAuthority(t *testing.T) {
	now := time.Date(2026, 8, 16, 16, 10, 0, 0, time.UTC)
	dir := t.TempDir()
	revocationPath := filepath.Join(dir, "revocations.json")
	distributionPath := filepath.Join(dir, "distribution.json")
	ackPath := filepath.Join(dir, "ack.json")
	emptyDigest := sessionLeafRevocationAuthorityDigest(map[[sha256.Size]byte]struct{}{})
	writeSessionLeafRevocationTest(t, revocationPath, "rev-001", nil)
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 1, emptyDigest, now.Add(10*time.Minute))
	setSessionLeafRevocationDistributionForTest(t, distributionPath, "instance-a", ackPath)
	runtime, err := newReloadableSessionLeafRevocationWithClock(revocationPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	identifier := sha256.Sum256([]byte("compromised-key"))
	writeSessionLeafRevocationTest(t, revocationPath, "rev-002", []string{hex.EncodeToString(identifier[:])})
	if _, err := runtime.Reload(); err == nil {
		t.Fatal("revocation candidate without matching distribution digest unexpectedly accepted")
	}
	if runtime.revokedCredential(identifier) {
		t.Fatal("invalid cross-file candidate changed F.24 LKG")
	}

	revoked := map[[sha256.Size]byte]struct{}{identifier: {}}
	revokedDigest := sessionLeafRevocationAuthorityDigest(revoked)
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 2, revokedDigest, now.Add(20*time.Minute))
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if !result.AuthorityChanged || !result.DistributionChanged || result.Generation != 2 || result.DistributionEpoch != 2 || !result.DistributionAckHealthy {
		t.Fatalf("paired publication=%+v", result)
	}
	if err := runtime.credentialAuthorizationError(identifier); err == nil {
		t.Fatal("revoked credential retained authority")
	}
}

func TestSessionLeafRevocationDistributionLeaseExpiryFailsClosedForProxyOnly(t *testing.T) {
	now := time.Date(2026, 8, 16, 16, 20, 0, 0, time.UTC)
	currentNow := now
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	revocationPath := filepath.Join(dir, "revocations.json")
	distributionPath := filepath.Join(dir, "distribution.json")
	ackPath := filepath.Join(dir, "ack.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings:        []sessionEdgePolicyBindingDefinition{{Prefixes: []string{"127.0.0.2/32"}, DNSNames: []string{"edge-a.astrahold.test"}}},
	})
	writeSessionLeafRevocationTest(t, revocationPath, "rev-001", nil)
	digest := sessionLeafRevocationAuthorityDigest(map[[sha256.Size]byte]struct{}{})
	validUntil := now.Add(5 * time.Minute)
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 1, digest, validUntil)
	setSessionLeafRevocationForTest(t, revocationPath)
	setSessionLeafRevocationDistributionForTest(t, distributionPath, "instance-a", ackPath)

	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return currentNow })
	if err != nil {
		t.Fatal(err)
	}
	attributor := &sessionSourceAttributor{edgePolicy: runtime}
	config, err := attributor.TLSConfig(&reloadableTLSCertificate{})
	if err != nil {
		t.Fatal(err)
	}
	proxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6900)}})
	if err != nil {
		t.Fatal(err)
	}
	spki := []byte("healthy-spki")
	leaf := &x509.Certificate{DNSNames: []string{"edge-a.astrahold.test"}, RawSubjectPublicKeyInfo: spki}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	if err := proxyConfig.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}

	currentNow = validUntil.Add(time.Second)
	expiredProxyConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.2", 6901)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := expiredProxyConfig.VerifyConnection(state); err == nil {
		t.Fatal("expired distribution lease allowed a fresh trusted-proxy handshake")
	}
	directConfig, err := config.GetConfigForClient(&tls.ClientHelloInfo{Conn: &sessionProxyTestConn{remote: tcpAddr("127.0.0.1", 6902)}})
	if err != nil {
		t.Fatal(err)
	}
	if directConfig != nil {
		t.Fatal("direct client unexpectedly gained proxy mTLS requirements after distribution expiry")
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:6900"
	request.Header.Set("X-Forwarded-For", "198.51.100.250")
	request.TLS = &state
	if _, err := attributor.sourceIP(request); err == nil {
		t.Fatal("existing trusted-proxy connection retained forwarding authority after distribution lease expiry")
	}
}

func TestSessionLeafRevocationDistributionAckFailureFencesAuthorityUntilRetry(t *testing.T) {
	now := time.Date(2026, 8, 16, 16, 30, 0, 0, time.UTC)
	dir := t.TempDir()
	revocationPath := filepath.Join(dir, "revocations.json")
	distributionPath := filepath.Join(dir, "distribution.json")
	ackPath := filepath.Join(dir, "ack.json")
	writeSessionLeafRevocationTest(t, revocationPath, "rev-001", nil)
	digest := sessionLeafRevocationAuthorityDigest(map[[sha256.Size]byte]struct{}{})
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 1, digest, now.Add(10*time.Minute))
	setSessionLeafRevocationDistributionForTest(t, distributionPath, "instance-a", ackPath)
	runtime, err := newReloadableSessionLeafRevocationWithClock(revocationPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(ackPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ackPath, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSessionLeafRevocationDistributionTest(t, distributionPath, 2, digest, now.Add(20*time.Minute))
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.DistributionAckHealthy {
		t.Fatal("ack publication failure unexpectedly reported healthy")
	}
	healthy := sha256.Sum256([]byte("healthy-key"))
	if err := runtime.credentialAuthorizationError(healthy); err == nil {
		t.Fatal("ack failure did not fail closed")
	}

	if err := os.Remove(ackPath); err != nil {
		t.Fatal(err)
	}
	retry, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if retry.DistributionChanged || !retry.DistributionAckHealthy || retry.DistributionEpoch != 2 {
		t.Fatalf("same-epoch ack retry=%+v", retry)
	}
	if err := runtime.credentialAuthorizationError(healthy); err != nil {
		t.Fatalf("authority did not recover after ack retry: %v", err)
	}
}

func setSessionLeafRevocationForTest(t *testing.T, path string) {
	t.Helper()
	previous := *sessionLoginTrustedProxyLeafRevocationFile
	*sessionLoginTrustedProxyLeafRevocationFile = path
	t.Cleanup(func() { *sessionLoginTrustedProxyLeafRevocationFile = previous })
}

func setSessionLeafRevocationDistributionForTest(t *testing.T, distributionPath, instanceID, ackPath string) {
	t.Helper()
	previousDistribution := *sessionLoginTrustedProxyLeafRevocationDistributionFile
	previousInstance := *sessionLoginTrustedProxyLeafRevocationInstanceID
	previousAck := *sessionLoginTrustedProxyLeafRevocationAckFile
	*sessionLoginTrustedProxyLeafRevocationDistributionFile = distributionPath
	*sessionLoginTrustedProxyLeafRevocationInstanceID = instanceID
	*sessionLoginTrustedProxyLeafRevocationAckFile = ackPath
	t.Cleanup(func() {
		*sessionLoginTrustedProxyLeafRevocationDistributionFile = previousDistribution
		*sessionLoginTrustedProxyLeafRevocationInstanceID = previousInstance
		*sessionLoginTrustedProxyLeafRevocationAckFile = previousAck
	})
}

func writeSessionLeafRevocationDistributionTest(t *testing.T, path string, epoch uint64, digest [sha256.Size]byte, validUntil time.Time) {
	t.Helper()
	data, err := json.Marshal(sessionLeafRevocationDistributionDefinition{
		SchemaVersion:             sessionLeafRevocationDistributionSchemaVersion,
		Epoch:                     epoch,
		RevocationAuthoritySHA256: hex.EncodeToString(digest[:]),
		ValidUntil:                validUntil.UTC().Truncate(time.Second).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
