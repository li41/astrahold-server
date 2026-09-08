package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func TestDurableRecoveryOutboxFailedRecordFencesVerifyBeforeProviderDeactivation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x6a)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{
		LoginID:           "alice",
		AccountID:         "acct-alice",
		CredentialVersion: 17,
		Eligible:          true,
	}
	challenge, err := reloadable.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	proof := readOutboxTestRecord(t, dir, challenge.RequestID).Proof

	before, ok := provider.sessionRecoveryOutboxSnapshot(challenge.RequestID)
	if !ok || !before.Active {
		t.Fatalf("provider challenge must be active before terminal delivery: snapshot=%+v ok=%v", before, ok)
	}

	// Reproduce the former authorization window deterministically: commit the
	// durable terminal state, but intentionally do not deactivate the provider's
	// in-memory challenge. Verify must still fail closed from durable truth.
	outbox.mu.Lock()
	record, exists := outbox.records[challenge.RequestID]
	if !exists {
		outbox.mu.Unlock()
		t.Fatal("missing durable recovery record")
	}
	record.DeliveryState = sessionRecoveryOutboxStateFailed
	record.Active = false
	record.Destination = ""
	record.Proof = ""
	record.NextAttemptAt = ""
	if err := outbox.writeRecordLocked(record); err != nil {
		outbox.mu.Unlock()
		t.Fatal(err)
	}
	outbox.records[challenge.RequestID] = record
	outbox.mu.Unlock()

	stillActive, ok := provider.sessionRecoveryOutboxSnapshot(challenge.RequestID)
	if !ok || !stillActive.Active {
		t.Fatalf("test did not preserve former race window: snapshot=%+v ok=%v", stillActive, ok)
	}
	if _, err := reloadable.Verify(context.Background(), challenge.RequestID, []byte(proof)); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("durable terminal state remained authorizing: %v", err)
	}
	after, ok := provider.sessionRecoveryOutboxSnapshot(challenge.RequestID)
	if !ok || after.Active {
		t.Fatalf("terminal durable fence did not retire provider challenge: snapshot=%+v ok=%v", after, ok)
	}
}
