package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func TestDurableRecoveryOutboxDropsExpiredRecordOnRestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x18)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := reloadable.Begin(context.Background(), accountrecovery.Subject{
		LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 17, Eligible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	record := readOutboxTestRecord(t, dir, challenge.RequestID)
	record.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	record.NextAttemptAt = record.ExpiresAt
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, record.DeliveryID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = outbox

	restartAdapter := &outboxTestHTTPAdapter{}
	restartProvider := newOutboxTestProvider(t, restartAdapter, now, 0x19)
	restartOutbox, err := newSessionRecoveryDurableOutbox(newOutboxTestConfig(dir), restartProvider, restartAdapter, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if restartOutbox.recordCount() != 0 {
		t.Fatalf("restored records=%d want=0", restartOutbox.recordCount())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired record still exists: err=%v", err)
	}
	if _, ok := restartProvider.sessionRecoveryOutboxSnapshot(challenge.RequestID); ok {
		t.Fatal("expired challenge restored")
	}
}

func TestDurableRecoveryOutboxRejectsNon0600LiveRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	adapter := &outboxTestHTTPAdapter{}
	provider := newOutboxTestProvider(t, adapter, now, 0x28)
	outbox := attachOutboxForTest(t, provider, newOutboxTestConfig(dir))
	reloadable, err := newReloadableSessionRecoveryProvider(provider, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := reloadable.Begin(context.Background(), accountrecovery.Subject{
		LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 23, Eligible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	record := readOutboxTestRecord(t, dir, challenge.RequestID)
	path := filepath.Join(dir, record.DeliveryID+".json")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	_ = outbox

	restartAdapter := &outboxTestHTTPAdapter{}
	restartProvider := newOutboxTestProvider(t, restartAdapter, now, 0x29)
	if _, err := newSessionRecoveryDurableOutbox(newOutboxTestConfig(dir), restartProvider, restartAdapter, time.Now); err == nil {
		t.Fatal("non-0600 live outbox record accepted")
	}
}
