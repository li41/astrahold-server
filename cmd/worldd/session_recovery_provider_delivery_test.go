package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

type recordingRecoveryDeliveryAdapter struct {
	deliveries []accountrecovery.Delivery
	err        error
}

func (a *recordingRecoveryDeliveryAdapter) Deliver(_ context.Context, delivery accountrecovery.Delivery) error {
	copyDelivery := delivery
	copyDelivery.Proof = append([]byte(nil), delivery.Proof...)
	a.deliveries = append(a.deliveries, copyDelivery)
	return a.err
}

func (a *recordingRecoveryDeliveryAdapter) Method() string   { return "recording-test" }
func (a *recordingRecoveryDeliveryAdapter) Revision() string { return "recording-test-001" }

func TestDeliveredSessionRecoveryProviderKnownUnknownAndConsumption(t *testing.T) {
	now := time.Date(2026, 8, 16, 6, 0, 0, 0, time.UTC)
	adapter := &recordingRecoveryDeliveryAdapter{}
	provider := mustNewDeliveredSessionRecoveryProvider(t, adapter, now)
	knownSubject := accountrecovery.Subject{
		LoginID:           "alice",
		AccountID:         "acct-alice",
		CredentialVersion: 7,
		Eligible:          true,
	}
	known, err := provider.Begin(context.Background(), knownSubject)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := provider.Begin(context.Background(), accountrecovery.Subject{
		LoginID:           "missing",
		AccountID:         "unmatched-recovery-account",
		CredentialVersion: 1,
		Eligible:          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !known.Valid() || !unknown.Valid() || known.RequestID == unknown.RequestID || !known.ExpiresAt.Equal(unknown.ExpiresAt) {
		t.Fatalf("challenge shapes known=%+v unknown=%+v", known, unknown)
	}
	if len(adapter.deliveries) != 1 {
		t.Fatalf("deliveries=%d want=1", len(adapter.deliveries))
	}
	delivery := adapter.deliveries[0]
	if delivery.RequestID != known.RequestID || delivery.Destination != "alice-inbox" || !delivery.ExpiresAt.Equal(known.ExpiresAt) || len(delivery.Proof) == 0 {
		t.Fatalf("delivery=%+v", delivery)
	}
	if _, err := provider.Verify(context.Background(), unknown.RequestID, delivery.Proof); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("unknown verify err=%v", err)
	}
	if _, err := provider.Verify(context.Background(), known.RequestID, []byte("wrong-proof")); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("wrong proof err=%v", err)
	}
	grant, err := provider.Verify(context.Background(), known.RequestID, delivery.Proof)
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccountID != knownSubject.AccountID || grant.CredentialVersion != knownSubject.CredentialVersion {
		t.Fatalf("grant=%+v", grant)
	}
	provider.Consume(context.Background(), known.RequestID)
	if _, err := provider.Verify(context.Background(), known.RequestID, delivery.Proof); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("consumed challenge err=%v", err)
	}
}

func TestDeliveredSessionRecoveryProviderFailureIsNonAuthorizingAcceptedChallenge(t *testing.T) {
	now := time.Date(2026, 8, 16, 6, 0, 0, 0, time.UTC)
	adapter := &recordingRecoveryDeliveryAdapter{err: accountrecovery.ErrDeliveryTransient}
	provider := mustNewDeliveredSessionRecoveryProvider(t, adapter, now)
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 7, Eligible: true}
	challenge, err := provider.Begin(context.Background(), subject)
	if err != nil {
		t.Fatalf("Begin should preserve generic accepted shape, err=%v", err)
	}
	if !challenge.Valid() || len(adapter.deliveries) != 1 {
		t.Fatalf("challenge=%+v deliveries=%d", challenge, len(adapter.deliveries))
	}
	if _, err := provider.Verify(context.Background(), challenge.RequestID, adapter.deliveries[0].Proof); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("failed-delivery challenge became redeemable, err=%v", err)
	}
}

func TestDeliveredSessionRecoveryProofIsCredentialGenerationBound(t *testing.T) {
	now := time.Date(2026, 8, 16, 6, 0, 0, 0, time.UTC)
	adapter := &recordingRecoveryDeliveryAdapter{}
	provider := mustNewDeliveredSessionRecoveryProvider(t, adapter, now)
	first, err := provider.Begin(context.Background(), accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 7, Eligible: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Begin(context.Background(), accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 8, Eligible: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.deliveries) != 2 {
		t.Fatalf("deliveries=%d", len(adapter.deliveries))
	}
	proofV7 := adapter.deliveries[0].Proof
	proofV8 := adapter.deliveries[1].Proof
	if bytes.Equal(proofV7, proofV8) {
		t.Fatal("delivery proof did not change with credential_version")
	}
	if _, err := provider.Verify(context.Background(), first.RequestID, proofV8); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("v8 proof authorized v7 challenge, err=%v", err)
	}
	if _, err := provider.Verify(context.Background(), second.RequestID, proofV7); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("v7 proof authorized v8 challenge, err=%v", err)
	}
}

func TestFilesystemRecoveryDeliveryAdapterWritesOnlyProofWithOwnerPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "inbox")
	adapter, err := newFilesystemRecoveryDeliveryAdapter(root, "filesystem-test-001")
	if err != nil {
		t.Fatal(err)
	}
	delivery := accountrecovery.Delivery{
		RequestID:   "opaque-request-id",
		Destination: "alice-ci-inbox",
		Proof:       []byte("high-entropy-proof-value"),
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
	}
	if err := adapter.Deliver(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, delivery.Destination+".proof")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(delivery.Proof)+"\n" {
		t.Fatalf("payload=%q", string(data))
	}
	if bytes.Contains(data, []byte(delivery.RequestID)) {
		t.Fatal("filesystem delivery persisted opaque request_id")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%#o want=0600", info.Mode().Perm())
	}

	bad := delivery
	bad.Destination = "../escape"
	if err := adapter.Deliver(context.Background(), bad); !errors.Is(err, accountrecovery.ErrDeliveryPermanent) {
		t.Fatalf("unsafe destination err=%v", err)
	}
}

func TestFilesystemRecoveryDeliveryAdapterRejectsBroadRootPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broad-inbox")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := newFilesystemRecoveryDeliveryAdapter(root, "filesystem-test-broad"); err == nil {
		t.Fatal("expected broad inbox permissions to be rejected")
	}
}

func TestLoadSchemaV2FilesystemRecoveryProvider(t *testing.T) {
	root := t.TempDir()
	key := bytes.Repeat([]byte{0x5a}, sessionRecoveryDeliveryProofKeyBytes)
	config := `{
  "schema_version": 2,
  "revision": "delivery-config-001",
  "proof_key_base64url": "` + base64.RawURLEncoding.EncodeToString(key) + `",
  "delivery": {
    "adapter": "filesystem-reference-v1",
    "revision": "filesystem-config-001",
    "inbox_dir": "` + filepath.ToSlash(filepath.Join(root, "inbox")) + `"
  },
  "subjects": [
    {"login_id": "alice", "destination": "alice-ci-inbox"}
  ]
}`
	path := filepath.Join(root, "provider.json")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := loadStaticSessionRecoveryProvider(path, 10*time.Minute, 5, time.Now, bytes.NewReader(deterministicRecoveryEntropy(256)))
	if err != nil {
		t.Fatal(err)
	}
	if provider.Method() != "hmac-sha256-generation-delivery" || provider.Revision() != "delivery-config-001" {
		t.Fatalf("method=%q revision=%q", provider.Method(), provider.Revision())
	}
}

func mustNewDeliveredSessionRecoveryProvider(t *testing.T, adapter accountrecovery.DeliveryAdapter, now time.Time) *staticSessionRecoveryProvider {
	t.Helper()
	key := bytes.Repeat([]byte{0x3c}, sessionRecoveryDeliveryProofKeyBytes)
	provider, err := newDeliveredSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion:     sessionRecoveryDeliveredProviderSchemaVersion,
		Revision:          "delivery-test-001",
		ProofKeyBase64URL: base64.RawURLEncoding.EncodeToString(key),
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:     "alice",
			Destination: "alice-inbox",
		}},
	}, adapter, 10*time.Minute, 3, func() time.Time { return now }, bytes.NewReader(deterministicRecoveryEntropy(1024)))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
