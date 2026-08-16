package main

import (
	"context"
	"testing"

	"github.com/li41/astrahold-server/internal/accountstore"
)

func TestDurableSessionLoginAuthenticatorAcceptsSchemaV3AndV4(t *testing.T) {
	for _, schema := range []uint16{accountstore.LegacySchemaVersion, accountstore.SchemaVersion} {
		t.Run(itoa(uint32(schema)), func(t *testing.T) {
			definition := durableAccountDefinitionForTest("password-1234", 1, "")
			definition.SchemaVersion = schema
			authenticator, err := newDurableSessionLoginAuthenticator(definition)
			if err != nil {
				t.Fatal(err)
			}
			grant, err := authenticator.Authenticate(context.Background(), "alice", []byte("password-1234"))
			if err != nil {
				t.Fatal(err)
			}
			if grant.AuthenticationSubject != "acct-alice" || grant.AuthenticationGeneration == "" {
				t.Fatalf("grant=%+v", grant)
			}
		})
	}
}

func TestDurableSessionLoginAuthenticatorRejectsMixedOnlineKDFPolicies(t *testing.T) {
	definition := durableAccountDefinitionForTest("alice-password", 1, "")
	definition.SchemaVersion = accountstore.SchemaVersion
	definition.Accounts = append(definition.Accounts, accountstore.Account{
		AccountID:         "acct-bob",
		LoginID:           "bob",
		PasswordArgon2ID:  testArgon2idPHC("bob-password", []byte("fedcba9876543210"), 128*1024, 4, 2),
		CredentialVersion: 1,
		CreatedAt:         "2026-08-16T00:00:00Z",
		PasswordChangedAt: "2026-08-16T00:00:00Z",
		CharacterID:       "bob-character",
	})
	if _, err := newDurableSessionLoginAuthenticator(definition); err == nil {
		t.Fatal("mixed online Argon2id costs must fail closed to preserve unknown-login KDF equivalence")
	}
}

func TestDurableSessionLoginAuthenticatorIgnoresRecoveryProofsForGameAuthority(t *testing.T) {
	definition := durableAccountDefinitionForTest("password-1234", 1, "")
	definition.SchemaVersion = accountstore.SchemaVersion
	definition.RecoveryGrants = []accountstore.RecoveryGrant{{
		RecoveryID:        "recovery-test",
		AccountID:         "acct-alice",
		CredentialVersion: 1,
		TokenSHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IssuedAt:          "2026-08-16T00:00:00Z",
		NotBefore:         "2026-08-16T00:00:00Z",
		ExpiresAt:         "2026-08-16T00:15:00Z",
	}}
	if err := accountstore.Validate(definition); err != nil {
		t.Fatal(err)
	}
	authenticator, err := newDurableSessionLoginAuthenticator(definition)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := authenticator.Authenticate(context.Background(), "alice", []byte("password-1234"))
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != "alice-character" || grant.RevocationScope != "" {
		t.Fatalf("recovery proof altered game grant: %+v", grant)
	}
}
