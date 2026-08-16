package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func TestStaticSessionRecoveryProviderKnownUnknownAndConsumption(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 0, 0, 0, time.UTC)
	code := []byte("F10-High-Entropy-Recovery-Code-For-Alice-0123456789")
	digest := sha256.Sum256(code)
	provider, err := newStaticSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion: sessionRecoveryProviderSchemaVersion,
		Revision:      "recovery-001",
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:            "alice",
			RecoveryCodeSHA256: hex.EncodeToString(digest[:]),
		}},
	}, 10*time.Minute, 3, func() time.Time { return now }, bytes.NewReader(deterministicRecoveryEntropy(256)))
	if err != nil {
		t.Fatal(err)
	}
	known, err := provider.Begin(context.Background(), accountrecovery.Subject{
		LoginID:           "alice",
		AccountID:         "acct-alice",
		CredentialVersion: 7,
		Eligible:          true,
	})
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
	if _, err := provider.Verify(context.Background(), unknown.RequestID, code); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("unknown verify err=%v", err)
	}
	if _, err := provider.Verify(context.Background(), known.RequestID, []byte("wrong-proof")); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("wrong proof err=%v", err)
	}
	grant, err := provider.Verify(context.Background(), known.RequestID, code)
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccountID != "acct-alice" || grant.CredentialVersion != 7 {
		t.Fatalf("grant=%+v", grant)
	}
	provider.Consume(context.Background(), known.RequestID)
	if _, err := provider.Verify(context.Background(), known.RequestID, code); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("consumed challenge err=%v", err)
	}
}

func TestStaticSessionRecoveryProviderAttemptAndExpiryBounds(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 0, 0, 0, time.UTC)
	code := []byte("F10-High-Entropy-Recovery-Code-For-Alice-0123456789")
	digest := sha256.Sum256(code)
	provider, err := newStaticSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion: sessionRecoveryProviderSchemaVersion,
		Revision:      "recovery-bounds",
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:            "alice",
			RecoveryCodeSHA256: hex.EncodeToString(digest[:]),
		}},
	}, time.Minute, 2, func() time.Time { return now }, bytes.NewReader(deterministicRecoveryEntropy(256)))
	if err != nil {
		t.Fatal(err)
	}
	subject := accountrecovery.Subject{LoginID: "alice", AccountID: "acct-alice", CredentialVersion: 2, Eligible: true}
	challenge, err := provider.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := provider.Verify(context.Background(), challenge.RequestID, []byte("wrong")); !errors.Is(err, accountrecovery.ErrRejected) {
			t.Fatalf("wrong attempt %d err=%v", i, err)
		}
	}
	if _, err := provider.Verify(context.Background(), challenge.RequestID, code); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("attempt-exhausted challenge err=%v", err)
	}

	expiring, err := provider.Begin(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := provider.Verify(context.Background(), expiring.RequestID, code); !errors.Is(err, accountrecovery.ErrRejected) {
		t.Fatalf("exact-expiry challenge err=%v", err)
	}
}
