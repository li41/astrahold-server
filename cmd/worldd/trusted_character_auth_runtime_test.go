package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStaticCredentialRevocationScopeIsStableForSameRecordAndChangesWithSecurityPolicy(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	token := []byte("scope-secret")
	base := trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "scope-a",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("scope-id", token, "scope-character", now, now.Add(time.Hour), time.Time{}),
		},
	}
	first, err := newStaticTrustedCharacterCredentialProvider(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newStaticTrustedCharacterCredentialProvider(base)
	if err != nil {
		t.Fatal(err)
	}
	firstScope := first.credentialsByID["scope-id"].revocationScope
	secondScope := second.credentialsByID["scope-id"].revocationScope
	if firstScope == "" || firstScope != secondScope {
		t.Fatalf("stable scope mismatch: first=%q second=%q", firstScope, secondScope)
	}

	changed := base
	changed.Revision = "scope-b"
	changed.Credentials = append([]trustedCharacterAuthCredential(nil), base.Credentials...)
	changed.Credentials[0].AllowActiveTakeover = true
	third, err := newStaticTrustedCharacterCredentialProvider(changed)
	if err != nil {
		t.Fatal(err)
	}
	if third.credentialsByID["scope-id"].revocationScope == firstScope {
		t.Fatal("security-relevant credential policy change must rotate revocation scope")
	}
}

func TestApplyTrustedCharacterAuthReloadPublishesFenceBeforeProviderAndRetainsLastKnownGoodOnFailure(t *testing.T) {
	now := time.Now().UTC()
	oldToken := []byte("runtime-old")
	newToken := []byte("runtime-new")
	path := filepath.Join(t.TempDir(), "trusted-auth.json")
	writeTrustedAuthDefinitionForRuntimeTest(t, path, trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "runtime-old-revision",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("runtime-old", oldToken, "runtime-character", time.Time{}, now.Add(time.Hour), time.Time{}),
		},
	})

	authenticator, runtime, revision, err := loadRuntimeTrustedCharacterAuthenticator(path, "127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	if authenticator == nil || runtime == nil || revision != "runtime-old-revision" {
		t.Fatalf("runtime load: authenticator=%v runtime=%v revision=%q", authenticator != nil, runtime != nil, revision)
	}
	if _, err := runtime.provider.Resolve(context.Background(), oldToken); err != nil {
		t.Fatalf("old provider rejected initial token: %v", err)
	}

	writeTrustedAuthDefinitionForRuntimeTest(t, path, trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "runtime-new-revision",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("runtime-new", newToken, "runtime-character", time.Time{}, now.Add(2*time.Hour), time.Time{}),
		},
	})

	replaceCalls := 0
	var installedScopes []string
	replace := func(scopes []string) int {
		replaceCalls++
		installedScopes = append(installedScopes[:0], scopes...)
		// During fence installation the old provider is still current. This is
		// intentional: stale old results may finish authentication but cannot publish.
		if current := runtime.provider.snapshot(); current == nil || current.revision != "runtime-old-revision" {
			t.Fatalf("provider swapped before scope fence: current=%v", current)
		}
		return 1
	}
	result, err := applyTrustedCharacterAuthReload(runtime, replace, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousRevision != "runtime-old-revision" || result.Revision != "runtime-new-revision" || result.ActiveScopes != 1 || result.RetiredPeers != 1 {
		t.Fatalf("reload result=%+v", result)
	}
	if replaceCalls != 1 || len(installedScopes) != 1 || installedScopes[0] == "" {
		t.Fatalf("scope replacement calls=%d scopes=%v", replaceCalls, installedScopes)
	}
	if current := runtime.provider.snapshot(); current == nil || current.revision != "runtime-new-revision" {
		t.Fatalf("provider did not swap after fence: current=%v", current)
	}
	if _, err := runtime.provider.Resolve(context.Background(), oldToken); err == nil {
		t.Fatal("removed old credential remained admissible after reload")
	}
	if _, err := runtime.provider.Resolve(context.Background(), newToken); err != nil {
		t.Fatalf("new credential rejected after reload: %v", err)
	}

	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applyTrustedCharacterAuthReload(runtime, replace, now); err == nil {
		t.Fatal("malformed reload must fail")
	}
	if replaceCalls != 1 {
		t.Fatalf("invalid reload must not replace transport scopes: calls=%d", replaceCalls)
	}
	if current := runtime.provider.snapshot(); current == nil || current.revision != "runtime-new-revision" {
		t.Fatalf("invalid reload discarded last-known-good provider: current=%v", current)
	}
	if _, err := runtime.provider.Resolve(context.Background(), newToken); err != nil {
		t.Fatalf("last-known-good credential stopped working after rejected reload: %v", err)
	}
}

func TestTrustedCharacterAuthRuntimeScopesAndNextBoundaryFollowLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	provider, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "runtime-boundaries",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("active", []byte("active-runtime"), "runtime-character", time.Time{}, now.Add(5*time.Minute), time.Time{}),
			lifecycleCredential("future", []byte("future-runtime"), "runtime-character", now.Add(time.Minute), now.Add(10*time.Minute), time.Time{}),
			lifecycleCredential("revoked", []byte("revoked-runtime"), "runtime-character", time.Time{}, now.Add(time.Hour), now),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scopes := activeTrustedCharacterAuthenticationScopes(provider, now)
	if len(scopes) != 1 || scopes[0] != provider.credentialsByID["active"].revocationScope {
		t.Fatalf("active scopes=%v", scopes)
	}
	boundary, ok := nextTrustedCharacterAuthenticationBoundary(provider, now)
	if !ok || !boundary.Equal(now.Add(time.Minute)) {
		t.Fatalf("next boundary=%v ok=%v want=%v", boundary, ok, now.Add(time.Minute))
	}
}

func TestRuntimeLoaderKeepsSchemaV1StartupCompatibilityWithoutHotReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trusted-auth-v1.json")
	writeTrustedAuthDefinitionForRuntimeTest(t, path, trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthLegacySchemaVersion,
		Revision:      "legacy-runtime-test",
		Credentials: []trustedCharacterAuthCredential{{
			TokenSHA256: credentialDigestHex([]byte("legacy-runtime")),
			CharacterID: "legacy-runtime-character",
		}},
	})
	authenticator, runtime, revision, err := loadRuntimeTrustedCharacterAuthenticator(path, "127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	if authenticator == nil || runtime != nil || revision != "legacy-runtime-test" {
		t.Fatalf("legacy runtime load: authenticator=%v runtime=%v revision=%q", authenticator != nil, runtime != nil, revision)
	}
}

func writeTrustedAuthDefinitionForRuntimeTest(t *testing.T, path string, definition trustedCharacterAuthDefinition) {
	t.Helper()
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
