package main

import (
	"testing"
	"time"
)

func TestStaticCredentialLifecycleChangeKeepsProofScopeUntilBoundary(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	token := []byte("scheduled-lifecycle-scope")
	baseCredential := lifecycleCredential(
		"scheduled-scope",
		token,
		"scheduled-character",
		time.Time{},
		now.Add(time.Hour),
		time.Time{},
	)
	first, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "scheduled-a",
		Credentials:   []trustedCharacterAuthCredential{baseCredential},
	})
	if err != nil {
		t.Fatal(err)
	}

	updatedCredential := baseCredential
	updatedCredential.ExpiresAt = now.Add(30 * time.Minute).Format(time.RFC3339)
	updatedCredential.RevokedAt = now.Add(20 * time.Minute).Format(time.RFC3339)
	second, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "scheduled-b",
		Credentials:   []trustedCharacterAuthCredential{updatedCredential},
	})
	if err != nil {
		t.Fatal(err)
	}

	firstScope := first.credentialsByID["scheduled-scope"].revocationScope
	secondScope := second.credentialsByID["scheduled-scope"].revocationScope
	if firstScope == "" || firstScope != secondScope {
		t.Fatalf("scheduled lifecycle update rotated proof scope early: first=%q second=%q", firstScope, secondScope)
	}
}
