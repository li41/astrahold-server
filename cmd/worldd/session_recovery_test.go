package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
	"github.com/li41/astrahold-server/internal/accountstore"
)

func TestPublicRecoveryRequestIsEnumerationResistantAndResetRetiresLiveBearer(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 30, 0, 0, time.UTC)
	runtime, path, recoveryCode := newRecoveryRuntimeForTest(t, now)
	retireTransitions := 0
	runtime.replaceScopes = func(scopes []string) int {
		if len(scopes) == 0 {
			retireTransitions++
			return 1
		}
		return 0
	}

	oldGrant, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte("Old-Recovery-Password-1234"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := runtime.Issue(context.Background(), oldGrant)
	if err != nil {
		t.Fatal(err)
	}

	knownResponse := requestRecoveryChallenge(t, runtime, "alice")
	unknownResponse := requestRecoveryChallenge(t, runtime, "missing-account")
	if knownResponse.Code != http.StatusAccepted || unknownResponse.Code != http.StatusAccepted {
		t.Fatalf("request statuses known=%d unknown=%d", knownResponse.Code, unknownResponse.Code)
	}
	if knownResponse.Header().Get("Cache-Control") != "no-store" || unknownResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("recovery request responses must be no-store")
	}
	var knownChallenge sessionRecoveryChallengeResponse
	var unknownChallenge sessionRecoveryChallengeResponse
	if err := json.Unmarshal(knownResponse.Body.Bytes(), &knownChallenge); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(unknownResponse.Body.Bytes(), &unknownChallenge); err != nil {
		t.Fatal(err)
	}
	if knownChallenge.RequestID == "" || unknownChallenge.RequestID == "" || knownChallenge.RequestID == unknownChallenge.RequestID || knownChallenge.ExpiresAt != unknownChallenge.ExpiresAt {
		t.Fatalf("public challenge shape known=%+v unknown=%+v", knownChallenge, unknownChallenge)
	}

	unknownReset := resetRecoveryPassword(t, runtime, unknownChallenge.RequestID, string(recoveryCode), "Unused-New-Password-1234")
	if unknownReset.Code != http.StatusUnauthorized || unknownReset.Body.String() != "{\"error\":\"invalid_recovery\"}\n" {
		t.Fatalf("unknown reset status=%d body=%q", unknownReset.Code, unknownReset.Body.String())
	}

	preResetClaim, err := runtime.recoveryProvider.Verify(context.Background(), knownChallenge.RequestID, recoveryCode)
	if err != nil {
		t.Fatal(err)
	}
	if preResetClaim.AccountID != "acct-alice" || preResetClaim.CredentialVersion != 1 {
		t.Fatalf("pre-reset claim=%+v", preResetClaim)
	}

	newPassword := "New-Recovery-Password-5678"
	knownReset := resetRecoveryPassword(t, runtime, knownChallenge.RequestID, string(recoveryCode), newPassword)
	if knownReset.Code != http.StatusNoContent {
		t.Fatalf("known reset status=%d body=%q", knownReset.Code, knownReset.Body.String())
	}
	if retireTransitions != 1 {
		t.Fatalf("retire transitions=%d want=1", retireTransitions)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.Credential)); err == nil {
		t.Fatal("recovery reset did not retire the pre-reset issued bearer")
	}

	definition, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Revision != 2 || definition.Accounts[0].CredentialVersion != 2 || len(definition.RecoveryGrants) != 0 {
		t.Fatalf("post-reset store revision=%d credential_version=%d recovery_grants=%d", definition.Revision, definition.Accounts[0].CredentialVersion, len(definition.RecoveryGrants))
	}
	if _, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte("Old-Recovery-Password-1234")); err == nil {
		t.Fatal("old password remained valid after public recovery reset")
	}
	if _, err := runtime.accountAuth.Authenticate(context.Background(), "alice", []byte(newPassword)); err != nil {
		t.Fatalf("new password rejected after public recovery reset: %v", err)
	}

	reuse := resetRecoveryPassword(t, runtime, knownChallenge.RequestID, string(recoveryCode), "Another-New-Password-9999")
	if reuse.Code != http.StatusUnauthorized || reuse.Body.String() != unknownReset.Body.String() {
		t.Fatalf("reuse response status=%d body=%q unknown_body=%q", reuse.Code, reuse.Body.String(), unknownReset.Body.String())
	}
	stalePHC := testArgon2idPHC("Stale-Generation-Password-1234", []byte("fedcba9876543210"), 64*1024, 3, 4)
	if _, err := runtime.resetDurableAccountFromRecovery(preResetClaim, stalePHC, now.Add(time.Minute)); !errors.Is(err, errSessionRecoveryRejected) {
		t.Fatalf("stale generation reset err=%v", err)
	}
}

func TestRecoveryResetFailsClosedWhenDiskRevisionOutrunsLiveAuthenticator(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 30, 0, 0, time.UTC)
	runtime, path, recoveryCode := newRecoveryRuntimeForTest(t, now)
	runtime.replaceScopes = func([]string) int { return 0 }
	challenge, err := runtime.recoveryProvider.Begin(context.Background(), runtime.accountAuth.snapshot().(*durableSessionLoginAuthenticator).RecoverySubject("alice"))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := runtime.recoveryProvider.Verify(context.Background(), challenge.RequestID, recoveryCode)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	definition.Revision++
	if err := accountstore.SaveIfRevision(path, 1, definition); err != nil {
		t.Fatal(err)
	}
	phc := testArgon2idPHC("Revision-Race-Password-1234", []byte("fedcba9876543210"), 64*1024, 3, 4)
	if _, err := runtime.resetDurableAccountFromRecovery(grant, phc, now); !errors.Is(err, errSessionRecoveryUnavailable) {
		t.Fatalf("revision drift reset err=%v", err)
	}
	loaded, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 2 || loaded.Accounts[0].CredentialVersion != 1 {
		t.Fatalf("fail-closed reset mutated store: revision=%d credential_version=%d", loaded.Revision, loaded.Accounts[0].CredentialVersion)
	}
}

func newRecoveryRuntimeForTest(t *testing.T, now time.Time) (*sessionLoginRuntime, string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := durableAccountDefinitionForTest("Old-Recovery-Password-1234", 1, "")
	legacyToken := sha256.Sum256([]byte("legacy-f9-recovery-proof"))
	definition.RecoveryGrants = []accountstore.RecoveryGrant{{
		RecoveryID:        "recovery-legacy-f9",
		AccountID:         "acct-alice",
		CredentialVersion: 1,
		TokenSHA256:       hex.EncodeToString(legacyToken[:]),
		IssuedAt:          "2026-08-16T04:00:00Z",
		NotBefore:         "2026-08-16T04:00:00Z",
		ExpiresAt:         "2026-08-16T04:45:00Z",
	}}
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
	runtime.recoveryGuard = newSessionLoginAbuseGuard(time.Minute, 100, func() time.Time { return now })
	recoveryCode := []byte("F10-High-Entropy-Recovery-Code-For-Alice-0123456789")
	digest := sha256.Sum256(recoveryCode)
	runtime.recoveryProvider, err = newStaticSessionRecoveryProvider(sessionRecoveryProviderDefinition{
		SchemaVersion: sessionRecoveryProviderSchemaVersion,
		Revision:      "recovery-http-test",
		Subjects: []sessionRecoveryProviderSubject{{
			LoginID:            "alice",
			RecoveryCodeSHA256: hex.EncodeToString(digest[:]),
		}},
	}, 10*time.Minute, 5, func() time.Time { return now }, bytes.NewReader(deterministicRecoveryEntropy(1024)))
	if err != nil {
		t.Fatal(err)
	}
	return runtime, path, recoveryCode
}

func requestRecoveryChallenge(t *testing.T, runtime *sessionLoginRuntime, loginID string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(sessionRecoveryChallengeRequest{LoginID: loginID})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/account/recovery/request", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(recorder, request)
	return recorder
}

func resetRecoveryPassword(t *testing.T, runtime *sessionLoginRuntime, requestID, proof, newPassword string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(sessionRecoveryResetRequest{
		RequestID:     requestID,
		RecoveryProof: proof,
		NewPassword:   newPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/account/recovery/reset", strings.NewReader(string(body)))
	recorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(recorder, request)
	return recorder
}

var _ = accountrecovery.ErrRejected
