package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountstore"
)

func TestDurableSessionLoginAuthenticatorTracksProofGeneration(t *testing.T) {
	definition := durableAccountDefinitionForTest("old-password", 1, "")
	authenticator, err := newDurableSessionLoginAuthenticator(definition)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := authenticator.Authenticate(context.Background(), "alice", []byte("old-password"))
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != "alice-character" || grant.AuthenticationSubject != "acct-alice" || grant.AuthenticationGeneration == "" {
		t.Fatalf("grant=%+v", grant)
	}
	if !authenticator.GenerationActive(grant.AuthenticationSubject, grant.AuthenticationGeneration) {
		t.Fatal("fresh account generation is not active")
	}

	disabled := definition
	disabled.Revision = 2
	disabled.Accounts = append([]accountstore.Account(nil), definition.Accounts...)
	disabled.Accounts[0].DisabledAt = "2026-08-16T00:01:00Z"
	disabled.Accounts[0].CredentialVersion = 2
	disabledAuthenticator, err := newDurableSessionLoginAuthenticator(disabled)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disabledAuthenticator.Authenticate(context.Background(), "alice", []byte("old-password")); err == nil {
		t.Fatal("disabled account must reject login")
	}
	if disabledAuthenticator.GenerationActive(grant.AuthenticationSubject, grant.AuthenticationGeneration) {
		t.Fatal("disabled account must retire the old proof generation")
	}
}

func TestDurableAccountReloadRevokesIssuedBearerAndStaleLoginGrant(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "accounts.json")
	initialDefinition := durableAccountDefinitionForTest("old-password", 1, "")
	if err := accountstore.Save(path, initialDefinition); err != nil {
		t.Fatal(err)
	}
	initial, err := loadDurableSessionLoginAuthenticator(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.accountPath = path
	runtime.accountAuth, err = newSessionAccountAuthRuntime(initial)
	if err != nil {
		t.Fatal(err)
	}
	retireCalls := 0
	runtime.replaceScopes = func(scopes []string) int {
		if len(scopes) == 0 {
			retireCalls++
			return 1
		}
		return 0
	}

	oldGrant, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte("old-password"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := runtime.Issue(context.Background(), oldGrant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential)); err != nil {
		t.Fatal(err)
	}

	rotated := durableAccountDefinitionForTest("new-password", 2, "")
	rotated.Accounts[0].CredentialVersion = 2
	rotated.Accounts[0].PasswordChangedAt = "2026-08-16T00:02:00Z"
	if err := accountstore.SaveIfRevision(path, 1, rotated); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.reloadDurableAccounts(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedBearers != 1 || result.RetiredPeers != 1 || retireCalls != 1 {
		t.Fatalf("reload=%+v retire_calls=%d", result, retireCalls)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential)); err == nil {
		t.Fatal("password rotation must revoke the previously issued bearer")
	}
	if _, err := runtime.Issue(context.Background(), oldGrant); !errors.Is(err, errSessionLoginRejected) {
		t.Fatalf("stale pre-reload login grant issue err=%v", err)
	}
	if _, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte("old-password")); err == nil {
		t.Fatal("old password remained valid after reload")
	}
	newGrant, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte("new-password"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Issue(context.Background(), newGrant); err != nil {
		t.Fatalf("new account generation failed issuance: %v", err)
	}
}

func TestDurableAccountReloadRejectsRevisionRollback(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := durableAccountDefinitionForTest("password-1234", 2, "")
	if err := accountstore.Save(path, definition); err != nil {
		t.Fatal(err)
	}
	authenticator, err := loadDurableSessionLoginAuthenticator(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.accountPath = path
	runtime.accountAuth, err = newSessionAccountAuthRuntime(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	runtime.replaceScopes = func([]string) int { return 0 }
	if _, err := runtime.reloadDurableAccounts(now); err == nil {
		t.Fatal("same durable revision must not reload")
	}
}

func durableAccountDefinitionForTest(password string, revision uint64, disabledAt string) accountstore.Definition {
	return accountstore.Definition{
		SchemaVersion: accountstore.SchemaVersion,
		Revision:      revision,
		Accounts: []accountstore.Account{{
			AccountID:           "acct-alice",
			LoginID:             "alice",
			PasswordArgon2ID:    testArgon2idPHC(password, []byte("0123456789abcdef"), 64*1024, 3, 4),
			CredentialVersion:   1,
			CreatedAt:           "2026-08-16T00:00:00Z",
			PasswordChangedAt:   "2026-08-16T00:00:00Z",
			DisabledAt:          disabledAt,
			CharacterID:         "alice-character",
			AllowActiveTakeover: true,
		}},
	}
}
