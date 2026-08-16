package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

type blockingRecoveryDeliveryAdapter struct {
	started    chan struct{}
	release    chan struct{}
	startOnce  sync.Once
	deliveries []accountrecovery.Delivery
	mu         sync.Mutex
}

func (a *blockingRecoveryDeliveryAdapter) Deliver(_ context.Context, delivery accountrecovery.Delivery) error {
	copyDelivery := delivery
	copyDelivery.Proof = append([]byte(nil), delivery.Proof...)
	a.mu.Lock()
	a.deliveries = append(a.deliveries, copyDelivery)
	a.mu.Unlock()
	a.startOnce.Do(func() { close(a.started) })
	<-a.release
	return nil
}

func (a *blockingRecoveryDeliveryAdapter) Method() string   { return "blocking-test" }
func (a *blockingRecoveryDeliveryAdapter) Revision() string { return "blocking-test-001" }

func TestReloadableSessionRecoveryProviderRetainsOldChallengeAndRetiresSecrets(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	firstAdapter := &recordingRecoveryDeliveryAdapter{}
	secondAdapter := &recordingRecoveryDeliveryAdapter{}
	first := mustNewReloadTestRecoveryProvider(t, "reload-provider-001", 0x11, 0x31, firstAdapter, now)
	second := mustNewReloadTestRecoveryProvider(t, "reload-provider-002", 0x22, 0x32, secondAdapter, now)
	reloadable, err := newReloadableSessionRecoveryProvider(first, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 7, Eligible: true}
	oldChallenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstAdapter.deliveries) != 1 {
		t.Fatalf("first deliveries=%d", len(firstAdapter.deliveries))
	}
	oldProof := append([]byte(nil), firstAdapter.deliveries[0].Proof...)

	result, err := reloadable.Replace(second)
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousGeneration != 1 || result.Generation != 2 || result.RetainedChallenges != 1 || result.RetiredChallenges != 0 {
		t.Fatalf("reload result=%+v", result)
	}
	if reloadable.Generation() != 2 || reloadable.Revision() != "reload-provider-002" {
		t.Fatalf("generation=%d revision=%q", reloadable.Generation(), reloadable.Revision())
	}
	if !bytes.Equal(first.proofKey[:], make([]byte, sessionRecoveryDeliveryProofKeyBytes)) {
		t.Fatal("retired provider proof key was not cleared")
	}
	grant, err := reloadable.Verify(context.Background(), oldChallenge.RequestID, oldProof)
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccountID != subject.AccountID || grant.CredentialVersion != subject.CredentialVersion {
		t.Fatalf("old grant=%+v", grant)
	}

	newChallenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondAdapter.deliveries) != 1 {
		t.Fatalf("second deliveries=%d", len(secondAdapter.deliveries))
	}
	newProof := secondAdapter.deliveries[0].Proof
	if bytes.Equal(oldProof, newProof) {
		t.Fatal("rotated proof key produced the old proof")
	}
	if _, err := reloadable.Verify(context.Background(), newChallenge.RequestID, newProof); err != nil {
		t.Fatalf("new generation verify err=%v", err)
	}
}

func TestReloadableSessionRecoveryProviderCutoverWaitsForInflightDelivery(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 15, 0, 0, time.UTC)
	blocking := &blockingRecoveryDeliveryAdapter{started: make(chan struct{}), release: make(chan struct{})}
	first := mustNewReloadTestRecoveryProvider(t, "reload-provider-blocking", 0x41, 0x51, blocking, now)
	second := mustNewReloadTestRecoveryProvider(t, "reload-provider-after", 0x42, 0x52, &recordingRecoveryDeliveryAdapter{}, now)
	reloadable, err := newReloadableSessionRecoveryProvider(first, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 3, Eligible: true}
	type beginResult struct {
		challenge accountrecovery.Challenge
		err       error
	}
	beginDone := make(chan beginResult, 1)
	go func() {
		challenge, err := reloadable.Begin(context.Background(), subject)
		beginDone <- beginResult{challenge: challenge, err: err}
	}()
	<-blocking.started

	replaceDone := make(chan error, 1)
	go func() {
		_, err := reloadable.Replace(second)
		replaceDone <- err
	}()
	select {
	case err := <-replaceDone:
		t.Fatalf("reload crossed in-flight delivery boundary: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(blocking.release)
	begin := <-beginDone
	if begin.err != nil {
		t.Fatal(begin.err)
	}
	if err := <-replaceDone; err != nil {
		t.Fatal(err)
	}
	blocking.mu.Lock()
	if len(blocking.deliveries) != 1 {
		blocking.mu.Unlock()
		t.Fatalf("deliveries=%d", len(blocking.deliveries))
	}
	proof := append([]byte(nil), blocking.deliveries[0].Proof...)
	blocking.mu.Unlock()
	if _, err := reloadable.Verify(context.Background(), begin.challenge.RequestID, proof); err != nil {
		t.Fatalf("in-flight old generation challenge lost after cutover: %v", err)
	}
}

func TestReloadableSessionRecoveryProviderBoundsRetiredGenerations(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 30, 0, 0, time.UTC)
	firstAdapter := &recordingRecoveryDeliveryAdapter{}
	first := mustNewReloadTestRecoveryProvider(t, "reload-provider-1", 0x61, 0x71, firstAdapter, now)
	reloadable, err := newReloadableSessionRecoveryProvider(first, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 9, Eligible: true}
	oldest, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	oldestProof := append([]byte(nil), firstAdapter.deliveries[0].Proof...)

	var secondChallenge accountrecovery.Challenge
	var secondProof []byte
	var lastResult sessionRecoveryProviderReloadResult
	for generation := 2; generation <= sessionRecoveryMaxRetiredProviderGenerations+2; generation++ {
		adapter := &recordingRecoveryDeliveryAdapter{}
		next := mustNewReloadTestRecoveryProvider(t, "reload-provider-next", byte(0x61+generation), byte(0x71+generation), adapter, now)
		lastResult, err = reloadable.Replace(next)
		if err != nil {
			t.Fatal(err)
		}
		if generation <= sessionRecoveryMaxRetiredProviderGenerations+1 {
			challenge, err := reloadable.Begin(context.Background(), subject)
			if err != nil {
				t.Fatal(err)
			}
			if generation == 2 {
				secondChallenge = challenge
				secondProof = append([]byte(nil), adapter.deliveries[0].Proof...)
			}
		}
	}
	if lastResult.RetiredChallenges < 1 {
		t.Fatalf("expected oldest generation retirement, result=%+v", lastResult)
	}
	if _, err := reloadable.Verify(context.Background(), oldest.RequestID, oldestProof); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("oldest generation remained redeemable after bound: %v", err)
	}
	if _, err := reloadable.Verify(context.Background(), secondChallenge.RequestID, secondProof); err != nil {
		t.Fatalf("next-oldest retained generation unexpectedly retired: %v", err)
	}
}

func TestReloadableSessionRecoveryProviderRejectsSchemaV1Replacement(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 45, 0, 0, time.UTC)
	current := mustNewReloadTestRecoveryProvider(t, "reload-provider-current", 0x21, 0x31, &recordingRecoveryDeliveryAdapter{}, now)
	reloadable, err := newReloadableSessionRecoveryProvider(current, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloadable.Replace(&staticSessionRecoveryProvider{revision: "schema-v1-like"}); !errors.Is(err, errSessionRecoveryReloadRestartOnly) {
		t.Fatalf("schema-v1 replacement err=%v", err)
	}
	if reloadable.Generation() != 1 || reloadable.Revision() != "reload-provider-current" {
		t.Fatalf("last-known-good changed: generation=%d revision=%q", reloadable.Generation(), reloadable.Revision())
	}
}

func mustNewReloadTestRecoveryProvider(t *testing.T, revision string, keyByte, entropyByte byte, adapter accountrecovery.DeliveryAdapter, now time.Time) *staticSessionRecoveryProvider {
	t.Helper()
	key := bytes.Repeat([]byte{keyByte}, sessionRecoveryDeliveryProofKeyBytes)
	provider, err := newDeliveredSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion:     sessionRecoveryDeliveredProviderSchemaVersion,
		Revision:          revision,
		ProofKeyBase64URL: base64.RawURLEncoding.EncodeToString(key),
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:     "alice",
			Destination: "alice-inbox",
		}},
	}, adapter, 10*time.Minute, 3, func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{entropyByte}, 2048)))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
