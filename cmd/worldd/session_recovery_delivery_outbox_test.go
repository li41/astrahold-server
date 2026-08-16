package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

type outboxTestHTTPAdapter struct {
	mu       sync.Mutex
	revision string
	result   func(int, accountrecovery.Delivery) error
	calls    []accountrecovery.Delivery
	retired  bool
}

func (a *outboxTestHTTPAdapter) Method() string { return sessionRecoveryHTTPAdapterMethod }
func (a *outboxTestHTTPAdapter) Revision() string {
	if a.revision == "" {
		return "outbox-test-http-001"
	}
	return a.revision
}
func (a *outboxTestHTTPAdapter) Deliver(_ context.Context, delivery accountrecovery.Delivery) error {
	a.mu.Lock()
	copyDelivery := delivery
	copyDelivery.Proof = append([]byte(nil), delivery.Proof...)
	a.calls = append(a.calls, copyDelivery)
	call := len(a.calls)
	result := a.result
	a.mu.Unlock()
	if result != nil {
		return result(call, copyDelivery)
	}
	return nil
}
func (a *outboxTestHTTPAdapter) Retire() {
	a.mu.Lock()
	a.retired = true
	a.mu.Unlock()
}
func (a *outboxTestHTTPAdapter) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}
func (a *outboxTestHTTPAdapter) firstCall() accountrecovery.Delivery {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.calls) == 0 {
		return accountrecovery.Delivery{}
	}
	copyDelivery := a.calls[0]
	copyDelivery.Proof = append([]byte(nil), copyDelivery.Proof...)
	return copyDelivery
}
func (a *outboxTestHTTPAdapter) isRetired() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.retired
}

func outboxTestEntropy(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i % 251) + 1)
	}
	return data
}

func newOutboxTestProvider(t *testing.T, adapter accountrecovery.DeliveryAdapter, now time.Time, keyByte byte) *staticSessionRecoveryProvider {
	t.Helper()
	key := bytes.Repeat([]byte{keyByte}, sessionRecoveryDeliveryProofKeyBytes)
	provider, err := newDeliveredSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion:     sessionRecoveryDeliveredProviderSchemaVersion,
		Revision:          "outbox-provider-test-001",
		ProofKeyBase64URL: base64.RawURLEncoding.EncodeToString(key),
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:     "alice",
			Destination: "alice@example.invalid",
		}},
	}, adapter, 10*time.Minute, 5, func() time.Time { return now }, bytes.NewReader(outboxTestEntropy(4096)))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func newOutboxTestConfig(dir string) sessionRecoveryOutboxConfig {
	return sessionRecoveryOutboxConfig{
		Dir:                 dir,
		MaxRecords:          16,
		MaxDeliveryAttempts: 4,
		RetryMin:            100 * time.Millisecond,
		RetryMax:            200 * time.Millisecond,
	}
}

func attachOutboxForTest(t *testing.T, provider *staticSessionRecoveryProvider, config sessionRecoveryOutboxConfig) *sessionRecoveryDurableOutbox {
	t.Helper()
	transport := provider.delivery
	outbox, err := newSessionRecoveryDurableOutbox(config, provider, transport, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	outbox.logf = func(string, ...any) {}
	provider.delivery = outbox
	return outbox
}

func waitOutboxTest(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for durable recovery outbox condition")
}

func readOutboxTestRecord(t *testing.T, dir, requestID string) sessionRecoveryOutboxRecord {
	t.Helper()
	path := filepath.Join(dir, recoveryDeliveryID(requestID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record sessionRecoveryOutboxRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestDurableRecoveryOutboxScrubsDeliveredProofAndRestoresChallenge(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x31)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 7, Eligible: true}
	challenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	pending := readOutboxTestRecord(t, dir, challenge.RequestID)
	if pending.DeliveryState != sessionRecoveryOutboxStatePending || pending.Proof == "" || pending.Destination != "alice@example.invalid" || !pending.Active {
		t.Fatalf("pending=%+v", pending)
	}
	proof := pending.Proof
	outbox.Start()
	defer outbox.Retire()
	waitOutboxTest(t, func() bool {
		return readOutboxTestRecord(t, dir, challenge.RequestID).DeliveryState == sessionRecoveryOutboxStateDelivered
	})
	delivered := readOutboxTestRecord(t, dir, challenge.RequestID)
	if delivered.Proof != "" || delivered.Destination != "" || !delivered.Active {
		t.Fatalf("delivered record retained delivery secrets: %+v", delivered)
	}
	path := filepath.Join(dir, delivered.DeliveryID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(proof)) || bytes.Contains(data, []byte("alice@example.invalid")) {
		t.Fatal("delivered challenge-only record retained proof or destination")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode=%#o want=0600", info.Mode().Perm())
	}
	outbox.Retire()

	// A cold restart may use a different provider proof key. The persisted
	// verifier/account generation must still make the pre-restart request usable.
	restartAdapter := &outboxTestHTTPAdapter{revision: "outbox-test-http-002"}
	restartProvider := newOutboxTestProvider(t, restartAdapter, now, 0x72)
	restartOutbox := attachOutboxForTest(t, restartProvider, newOutboxTestConfig(dir))
	restartOutbox.Start()
	defer restartOutbox.Retire()
	restartReloadable, err := newReloadableSessionRecoveryProvider(restartProvider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// A cold-restored challenge must be seeded into generation 1 routing so a
	// subsequent F.14 cutover does not orphan it before its TTL expires.
	postRestartAdapter := &outboxTestHTTPAdapter{revision: "outbox-test-http-003"}
	postRestartProvider := newOutboxTestProvider(t, postRestartAdapter, now, 0x73)
	if _, err := restartReloadable.Replace(postRestartProvider); err != nil {
		t.Fatal(err)
	}
	grant, err := restartReloadable.Verify(context.Background(), challenge.RequestID, []byte(proof))
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccountID != subject.AccountID || grant.CredentialVersion != subject.CredentialVersion {
		t.Fatalf("grant=%+v", grant)
	}
}

func TestDurableRecoveryOutboxReplaysTransientAfterRestart(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	now := time.Now().UTC()
	firstAdapter := &outboxTestHTTPAdapter{result: func(int, accountrecovery.Delivery) error {
		return accountrecovery.ErrDeliveryTransient
	}}
	provider := newOutboxTestProvider(t, firstAdapter, now, 0x41)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := reloadable.Begin(context.Background(), accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 9, Eligible: true})
	if err != nil {
		t.Fatal(err)
	}
	pending := readOutboxTestRecord(t, dir, challenge.RequestID)
	proof := pending.Proof
	outbox.Start()
	waitOutboxTest(t, func() bool { return firstAdapter.callCount() >= 1 })
	outbox.Retire()
	firstCall := firstAdapter.firstCall()
	if recoveryDeliveryID(firstCall.RequestID) != pending.DeliveryID {
		t.Fatal("initial delivery identity does not match durable record")
	}

	secondAdapter := &outboxTestHTTPAdapter{revision: "outbox-test-http-restart"}
	restartProvider := newOutboxTestProvider(t, secondAdapter, now, 0x62)
	restartOutbox := attachOutboxForTest(t, restartProvider, newOutboxTestConfig(dir))
	restartReloadable, err := newReloadableSessionRecoveryProvider(restartProvider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	restartOutbox.Start()
	defer restartOutbox.Retire()
	waitOutboxTest(t, func() bool { return secondAdapter.callCount() >= 1 })
	secondCall := secondAdapter.firstCall()
	if secondCall.RequestID != challenge.RequestID || string(secondCall.Proof) != proof || recoveryDeliveryID(secondCall.RequestID) != pending.DeliveryID {
		t.Fatalf("replayed delivery changed identity/material: %+v", secondCall)
	}
	waitOutboxTest(t, func() bool {
		return readOutboxTestRecord(t, dir, challenge.RequestID).DeliveryState == sessionRecoveryOutboxStateDelivered
	})
	if _, err := restartReloadable.Verify(context.Background(), challenge.RequestID, []byte(proof)); err != nil {
		t.Fatalf("restored challenge verify: %v", err)
	}
}

func TestDurableRecoveryOutboxBackpressurePreservesGenericNonAuthorizingChallenge(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x51)
	config := newOutboxTestConfig(dir)
	config.MaxRecords = 1
	outbox := attachOutboxForTest(t, provider, config)
	_ = outbox
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 11, Eligible: true}
	first, err := reloadable.Begin(context.Background(), subject)
	if err != nil || !first.Valid() {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := reloadable.Begin(context.Background(), subject)
	if err != nil || !second.Valid() {
		t.Fatalf("backpressured request must preserve accepted shape: challenge=%+v err=%v", second, err)
	}
	proof := provider.deliveredProof(subject)
	defer clear(proof)
	if _, err := reloadable.Verify(context.Background(), second.RequestID, proof); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("backpressured challenge became authorizing: %v", err)
	}
	if _, err := reloadable.Verify(context.Background(), first.RequestID, proof); err != nil {
		t.Fatalf("durably enqueued challenge rejected: %v", err)
	}
}

func TestDurableRecoveryOutboxVerificationAttemptsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x33)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := reloadable.Begin(context.Background(), accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 3, Eligible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloadable.Verify(context.Background(), challenge.RequestID, []byte("wrong-proof")); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("wrong proof err=%v", err)
	}
	record := readOutboxTestRecord(t, dir, challenge.RequestID)
	if record.VerificationAttempts != 1 {
		t.Fatalf("verification_attempts=%d want=1", record.VerificationAttempts)
	}
	_ = outbox

	restartAdapter := &outboxTestHTTPAdapter{}
	restartProvider := newOutboxTestProvider(t, restartAdapter, now, 0x44)
	restartOutbox := attachOutboxForTest(t, restartProvider, newOutboxTestConfig(dir))
	_ = restartOutbox
	snapshot, ok := restartProvider.sessionRecoveryOutboxSnapshot(challenge.RequestID)
	if !ok || snapshot.VerificationAttempts != 1 {
		t.Fatalf("snapshot=%+v ok=%v", snapshot, ok)
	}
}

func TestDurableRecoveryOutboxF14TransportSwapKeepsPendingRecord(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	now := time.Now().UTC()
	oldAdapter := &outboxTestHTTPAdapter{revision: "old-relay", result: func(int, accountrecovery.Delivery) error {
		return accountrecovery.ErrDeliveryTransient
	}}
	provider := newOutboxTestProvider(t, oldAdapter, now, 0x22)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 5, Eligible: true}
	challenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	proof := readOutboxTestRecord(t, dir, challenge.RequestID).Proof
	outbox.Start()
	defer outbox.Retire()
	waitOutboxTest(t, func() bool { return oldAdapter.callCount() >= 1 })

	newAdapter := &outboxTestHTTPAdapter{revision: "new-relay"}
	next := newOutboxTestProvider(t, newAdapter, now, 0x77)
	result, err := reloadable.Replace(next)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation != 2 || next.delivery != outbox || !oldAdapter.isRetired() {
		t.Fatalf("reload result=%+v shared=%v retired=%v", result, next.delivery == outbox, oldAdapter.isRetired())
	}
	waitOutboxTest(t, func() bool { return newAdapter.callCount() >= 1 })
	waitOutboxTest(t, func() bool {
		return readOutboxTestRecord(t, dir, challenge.RequestID).DeliveryState == sessionRecoveryOutboxStateDelivered
	})
	if _, err := reloadable.Verify(context.Background(), challenge.RequestID, []byte(proof)); err != nil {
		t.Fatalf("pre-cutover challenge rejected after transport swap: %v", err)
	}
}

func TestDurableRecoveryOutboxPermanentFailureScrubsAndDeactivates(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{result: func(int, accountrecovery.Delivery) error {
		return accountrecovery.ErrDeliveryPermanent
	}}
	provider := newOutboxTestProvider(t, adapter, now, 0x61)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 13, Eligible: true}
	challenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	proof := readOutboxTestRecord(t, dir, challenge.RequestID).Proof
	outbox.Start()
	defer outbox.Retire()
	waitOutboxTest(t, func() bool {
		return readOutboxTestRecord(t, dir, challenge.RequestID).DeliveryState == sessionRecoveryOutboxStateFailed
	})
	failed := readOutboxTestRecord(t, dir, challenge.RequestID)
	if failed.Active || failed.Proof != "" || failed.Destination != "" {
		t.Fatalf("failed=%+v", failed)
	}
	if _, err := reloadable.Verify(context.Background(), challenge.RequestID, []byte(proof)); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("terminal delivery failure remained authorizing: %v", err)
	}
}

func TestDurableRecoveryOutboxRejectsUnsafeRootAndCorruptRecord(t *testing.T) {
	dir := t.TempDir()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, time.Now().UTC(), 0x70)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := newSessionRecoveryDurableOutbox(newOutboxTestConfig(dir), provider, adapter, time.Now); err == nil {
		t.Fatal("broad outbox permissions accepted")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSessionRecoveryDurableOutbox(newOutboxTestConfig(dir), provider, adapter, time.Now); err == nil {
		t.Fatal("corrupt outbox record accepted")
	}
}
