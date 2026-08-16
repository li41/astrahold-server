package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

func TestStaticSessionLoginAuthenticatorSelectsServerOwnedClaims(t *testing.T) {
	secret := []byte("high-entropy-login-secret-for-tests")
	digest := sha256.Sum256(secret)
	authenticator, err := newStaticSessionLoginAuthenticator(sessionLoginDefinition{
		SchemaVersion: sessionLoginSchemaVersion,
		Revision:      "accounts-test-001",
		Accounts: []sessionLoginAccount{{
			LoginID:             "account-1",
			LoginSecretSHA256:   hex.EncodeToString(digest[:]),
			CharacterID:         "character-account-1",
			AllowActiveTakeover: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := authenticator.Authenticate(context.Background(), "account-1", secret)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != "character-account-1" || grant.Identity.Assurance != characteridentity.AssuranceTrusted {
		t.Fatalf("grant identity=%+v", grant.Identity)
	}
	if !grant.AllowActiveTakeover {
		t.Fatal("server-side takeover claim was not preserved")
	}
	if grant.RevocationScope != "" {
		t.Fatalf("account login grant must not preselect session scope: %q", grant.RevocationScope)
	}
	if _, err := authenticator.Authenticate(context.Background(), "account-1", []byte("wrong")); err == nil {
		t.Fatal("wrong login secret must fail")
	}
}

func TestStaticSessionLoginAuthenticatorRejectsDuplicateLoginID(t *testing.T) {
	digest := sha256.Sum256([]byte("secret"))
	_, err := newStaticSessionLoginAuthenticator(sessionLoginDefinition{
		SchemaVersion: sessionLoginSchemaVersion,
		Revision:      "dup",
		Accounts: []sessionLoginAccount{
			{LoginID: "same", LoginSecretSHA256: hex.EncodeToString(digest[:]), CharacterID: "char-a"},
			{LoginID: "same", LoginSecretSHA256: hex.EncodeToString(digest[:]), CharacterID: "char-b"},
		},
	})
	if err == nil {
		t.Fatal("duplicate login_id must fail")
	}
}

func TestIssuedSessionCredentialPublishesFenceBeforeProvider(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	currentCountAtFence := -1
	var publishedScopes []string
	runtime.replaceScopes = func(scopes []string) int {
		currentCountAtFence = len(runtime.provider.snapshot().credentials)
		publishedScopes = append([]string(nil), scopes...)
		return 0
	}
	binding, err := characteridentity.NewTrusted("character-issued")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := runtime.Issue(context.Background(), sessioncredential.Grant{
		Identity:            binding,
		AllowActiveTakeover: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !issued.Valid() {
		t.Fatalf("issued=%+v", issued)
	}
	if len(issued.Credential) != 43 {
		t.Fatalf("credential length=%d want=43", len(issued.Credential))
	}
	if currentCountAtFence != 0 {
		t.Fatalf("provider published before scope fence: count=%d", currentCountAtFence)
	}
	if len(publishedScopes) != 1 || !strings.HasPrefix(publishedScopes[0], issuedSessionScopePrefix) {
		t.Fatalf("published scopes=%v", publishedScopes)
	}
	grant, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential))
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != binding.ID || !grant.AllowActiveTakeover || grant.RevocationScope != publishedScopes[0] {
		t.Fatalf("resolved grant=%+v", grant)
	}
	if !issued.ExpiresAt.Equal(now.Add(runtime.ttl)) {
		t.Fatalf("expires=%s want=%s", issued.ExpiresAt, now.Add(runtime.ttl))
	}
}

func TestIssuedSessionCredentialRevocationFencesBeforeProviderRemoval(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.replaceScopes = func([]string) int { return 0 }
	binding, _ := characteridentity.NewTrusted("character-revoke")
	issued, err := runtime.Issue(context.Background(), sessioncredential.Grant{Identity: binding})
	if err != nil {
		t.Fatal(err)
	}

	providerCountAtFence := -1
	runtime.replaceScopes = func(scopes []string) int {
		providerCountAtFence = len(runtime.provider.snapshot().credentials)
		if len(scopes) != 0 {
			t.Fatalf("revoke scopes=%v want empty", scopes)
		}
		return 1
	}
	revoked, err := runtime.Revoke(context.Background(), []byte(issued.Credential))
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("known issued credential was not revoked")
	}
	if providerCountAtFence != 1 {
		t.Fatalf("provider removed before transport fence: count=%d", providerCountAtFence)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential)); err == nil {
		t.Fatal("revoked bearer must stop resolving")
	}
}

func TestIssuedSessionCredentialExpiresAtExactCutoff(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	clock := now
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return clock })
	calls := 0
	runtime.replaceScopes = func(scopes []string) int {
		calls++
		if calls == 1 && len(scopes) != 1 {
			t.Fatalf("issue scopes=%v", scopes)
		}
		if calls == 2 && len(scopes) != 0 {
			t.Fatalf("expiry scopes=%v", scopes)
		}
		if calls == 2 {
			return 1
		}
		return 0
	}
	binding, _ := characteridentity.NewTrusted("character-expiry")
	issued, err := runtime.Issue(context.Background(), sessioncredential.Grant{Identity: binding})
	if err != nil {
		t.Fatal(err)
	}
	clock = issued.ExpiresAt
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential)); err == nil {
		t.Fatal("credential must reject at exact expires_at cutoff")
	}
	if retired := runtime.expireAt(clock); retired != 1 {
		t.Fatalf("retired=%d want=1", retired)
	}
	if len(runtime.provider.snapshot().credentials) != 0 {
		t.Fatal("expired credential record was not pruned")
	}
}

func TestSessionLoginHTTPLoginThenLogout(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	secretText := "account-login-secret"
	digest := sha256.Sum256([]byte(secretText))
	authenticator, err := newStaticSessionLoginAuthenticator(sessionLoginDefinition{
		SchemaVersion: sessionLoginSchemaVersion,
		Revision:      "accounts-http-001",
		Accounts: []sessionLoginAccount{{
			LoginID:           "alice",
			LoginSecretSHA256: hex.EncodeToString(digest[:]),
			CharacterID:       "alice-character",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.accountAuth, err = newSessionAccountAuthRuntime(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	var latestScopes []string
	runtime.replaceScopes = func(scopes []string) int {
		latestScopes = append([]string(nil), scopes...)
		return 0
	}

	loginBody := `{"login_id":"alice","login_secret":"account-login-secret"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(loginBody))
	recorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", recorder.Header().Get("Cache-Control"))
	}
	var response sessionLoginResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SessionCredential == "" || len(latestScopes) != 1 {
		t.Fatalf("response=%+v scopes=%v", response, latestScopes)
	}
	grant, err := runtime.provider.Resolve(context.Background(), []byte(response.SessionCredential))
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != "alice-character" {
		t.Fatalf("character=%q", grant.Identity.ID)
	}

	logout := httptest.NewRequest(http.MethodPost, "/v1/session/logout", nil)
	logout.Header.Set("Authorization", "Bearer "+response.SessionCredential)
	logoutRecorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(logoutRecorder, logout)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if len(latestScopes) != 0 {
		t.Fatalf("logout scopes=%v want empty", latestScopes)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(response.SessionCredential)); err == nil {
		t.Fatal("logout must revoke issued bearer")
	}
}

func TestSessionLoginHTTPRejectsUnknownFieldsAndWrongSecret(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	secret := sha256.Sum256([]byte("right"))
	authenticator, err := newStaticSessionLoginAuthenticator(sessionLoginDefinition{
		SchemaVersion: sessionLoginSchemaVersion,
		Revision:      "accounts-http-002",
		Accounts: []sessionLoginAccount{{
			LoginID:           "alice",
			LoginSecretSHA256: hex.EncodeToString(secret[:]),
			CharacterID:       "alice-character",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.accountAuth, err = newSessionAccountAuthRuntime(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	runtime.replaceScopes = func([]string) int { return 0 }

	cases := map[string]struct {
		body string
		want int
	}{
		"unknown-field": {`{"login_id":"alice","login_secret":"right","character_id":"client-choice"}`, http.StatusBadRequest},
		"wrong-secret":  {`{"login_id":"alice","login_secret":"wrong"}`, http.StatusUnauthorized},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(testCase.body))
			recorder := httptest.NewRecorder()
			runtime.handler().ServeHTTP(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}
}

func TestIssuedSessionProviderStoresDigestNotPlaintext(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.replaceScopes = func([]string) int { return 0 }
	binding, _ := characteridentity.NewTrusted("character-digest")
	issued, err := runtime.Issue(context.Background(), sessioncredential.Grant{Identity: binding})
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(issued.Credential))
	for _, entry := range runtime.provider.snapshot().credentials {
		if entry.tokenDigest != want {
			t.Fatalf("stored digest=%x want=%x", entry.tokenDigest, want)
		}
	}
}

func newTestSessionLoginRuntime(t *testing.T, now func() time.Time) *sessionLoginRuntime {
	t.Helper()
	provider, err := newReloadableTrustedCharacterCredentialProvider(newEmptyIssuedSessionCredentialProvider(now))
	if err != nil {
		t.Fatal(err)
	}
	randomBytes := bytes.Repeat([]byte{0x42}, issuedSessionCredentialRandomBytes*8)
	return &sessionLoginRuntime{
		provider: provider,
		ttl:      15 * time.Minute,
		now:      now,
		random:   bytes.NewReader(randomBytes),
		changed:  make(chan struct{}, 1),
	}
}
