package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func writeTestRecoveryRelayCredential(t *testing.T, dir, value string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "relay.token")
	if err := os.WriteFile(path, []byte(value+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestRecoveryRelayCA(t *testing.T, dir string, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test relay has no certificate")
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	path := filepath.Join(dir, "relay-ca.pem")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustNewTestHTTPRecoveryDeliveryAdapter(t *testing.T, server *httptest.Server, credential string, maxAttempts int) *httpRecoveryDeliveryAdapter {
	t.Helper()
	dir := t.TempDir()
	credentialPath := writeTestRecoveryRelayCredential(t, dir, credential, 0o600)
	caPath := writeTestRecoveryRelayCA(t, dir, server)
	adapter, err := newHTTPRecoveryDeliveryAdapter(server.URL, credentialPath, caPath, "http-test-001", 500*time.Millisecond, maxAttempts, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestHTTPRecoveryDeliveryAdapterRetriesTransientWithStableIdempotencyAndSafeLogs(t *testing.T) {
	credential := "f13-relay-credential-0123456789"
	var mu sync.Mutex
	var bodies [][]byte
	var ids []string
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Errorf("tls_version=%v", r.TLS)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+credential {
			t.Errorf("authorization=%q", got)
		}
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), body.Bytes()...))
		ids = append(ids, r.Header.Get("Idempotency-Key"))
		attempts++
		current := attempts
		mu.Unlock()
		if r.Header.Get("X-Astrahold-Delivery-ID") != r.Header.Get("Idempotency-Key") {
			t.Error("delivery correlation header differs from idempotency key")
		}
		if current == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	adapter := mustNewTestHTTPRecoveryDeliveryAdapter(t, server, credential, 3)
	var logs bytes.Buffer
	adapter.logf = func(format string, args ...any) { fmt.Fprintf(&logs, format, args...) }
	delivery := accountrecovery.Delivery{
		RequestID:   "opaque-request-id",
		Destination: "alice@example.invalid",
		Proof:       []byte("proof-secret-123"),
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
	}
	if err := adapter.Deliver(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Fatalf("attempts=%d want=2", attempts)
	}
	if ids[0] == "" || ids[0] != ids[1] || ids[0] != recoveryDeliveryID(delivery.RequestID) {
		t.Fatalf("ids=%v", ids)
	}
	if !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatal("retry body changed")
	}
	var payload httpRecoveryDeliveryPayload
	if err := json.Unmarshal(bodies[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DeliveryID != ids[0] || payload.Destination != delivery.Destination || payload.Proof != string(delivery.Proof) {
		t.Fatalf("payload=%+v", payload)
	}
	if bytes.Contains(bodies[0], []byte(delivery.RequestID)) || bytes.Contains(bodies[0], []byte(credential)) {
		t.Fatal("relay payload leaked raw request id or bearer credential")
	}
	for _, secret := range []string{delivery.RequestID, delivery.Destination, string(delivery.Proof), credential} {
		if bytes.Contains(logs.Bytes(), []byte(secret)) {
			t.Fatalf("logs leaked %q: %s", secret, logs.String())
		}
	}
	if !bytes.Contains(logs.Bytes(), []byte("outcome=success attempts=2 status_class=2xx")) {
		t.Fatalf("logs=%s", logs.String())
	}
}

func TestHTTPRecoveryDeliveryAdapterPermanentFailureDoesNotRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	adapter := mustNewTestHTTPRecoveryDeliveryAdapter(t, server, "credential-0123456789", 3)
	adapter.logf = func(string, ...any) {}
	err := adapter.Deliver(context.Background(), accountrecovery.Delivery{RequestID: "request", Destination: "destination", Proof: []byte("proof"), ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if !errors.Is(err, accountrecovery.ErrDeliveryPermanent) {
		t.Fatalf("err=%v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want=1", attempts)
	}
}

func TestHTTPRecoveryDeliveryAdapterTransientFailureExhaustsRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	adapter := mustNewTestHTTPRecoveryDeliveryAdapter(t, server, "credential-0123456789", 3)
	adapter.logf = func(string, ...any) {}
	err := adapter.Deliver(context.Background(), accountrecovery.Delivery{RequestID: "request", Destination: "destination", Proof: []byte("proof"), ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if !errors.Is(err, accountrecovery.ErrDeliveryTransient) {
		t.Fatalf("err=%v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want=3", attempts)
	}
}

func TestLoadRecoveryDeliveryCredentialRequiresOwnerOnlyRegularFile(t *testing.T) {
	dir := t.TempDir()
	broad := writeTestRecoveryRelayCredential(t, dir, "credential-0123456789", 0o644)
	if _, err := loadRecoveryDeliveryCredential(broad); err == nil {
		t.Fatal("expected broad credential permissions rejection")
	}
	if err := os.Chmod(broad, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := loadRecoveryDeliveryCredential(broad)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	if string(data) != "credential-0123456789" {
		t.Fatalf("credential=%q", data)
	}
}

func TestValidateRecoveryDeliveryEndpointRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/deliver",
		"https://user:pass@example.invalid/deliver",
		"https://example.invalid/deliver#fragment",
	} {
		if _, err := validateRecoveryDeliveryEndpoint(raw); err == nil {
			t.Fatalf("endpoint %q should be rejected", raw)
		}
	}
}

func TestLoadSchemaV2HTTPRecoveryProvider(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	dir := t.TempDir()
	credentialPath := writeTestRecoveryRelayCredential(t, dir, "credential-0123456789", 0o600)
	caPath := writeTestRecoveryRelayCA(t, dir, server)
	key := bytes.Repeat([]byte{0x5a}, sessionRecoveryDeliveryProofKeyBytes)
	config := fmt.Sprintf(`{
  "schema_version": 2,
  "revision": "delivery-http-config-001",
  "proof_key_base64url": "%s",
  "delivery": {
    "adapter": "https-json-v1",
    "revision": "relay-config-001",
    "endpoint": %q,
    "credential_file": %q,
    "ca_file": %q,
    "request_timeout": "500ms",
    "max_attempts": 3,
    "retry_backoff": "10ms"
  },
  "subjects": [
    {"login_id": "alice", "destination": "alice@example.invalid"}
  ]
}`,
		base64.RawURLEncoding.EncodeToString(key), server.URL, filepath.ToSlash(credentialPath), filepath.ToSlash(caPath))
	path := filepath.Join(dir, "provider.json")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := loadStaticSessionRecoveryProvider(path, 10*time.Minute, 5, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x42}, 2048)))
	if err != nil {
		t.Fatal(err)
	}
	if provider.Method() != "hmac-sha256-generation-delivery" || provider.Revision() != "delivery-http-config-001" {
		t.Fatalf("method=%q revision=%q", provider.Method(), provider.Revision())
	}
	challenge, err := provider.Begin(context.Background(), accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 1, Eligible: true})
	if err != nil || !challenge.Valid() {
		t.Fatalf("challenge=%+v err=%v", challenge, err)
	}
}
