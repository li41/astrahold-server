package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSessionLoginRuntimeServesTLS13AndIssuesGameCredential(t *testing.T) {
	secretText := "integration-high-entropy-login-secret"
	accountFile := writeSessionLoginAccountFile(t, "integration-001", "integration", secretText, "integration-character", true)
	certFile, keyFile, roots := writeTrustedTLSCertificate(t)
	configureSessionLoginFlags(t, accountFile, certFile, keyFile)

	runtime, err := loadSessionLoginRuntime("127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	var scopeMu sync.Mutex
	var scopes []string
	replaceScopes := func(next []string) int {
		scopeMu.Lock()
		scopes = append([]string(nil), next...)
		scopeMu.Unlock()
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runIssuedSessionCredentialRuntime(ctx, nil, runtime, replaceScopes, nil)
		close(done)
	}()

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:    roots,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
	}}}
	loginURL := "https://" + runtime.Addr().String() + "/v1/session/login"
	response, err := client.Post(loginURL, "application/json", strings.NewReader(`{"login_id":"integration","login_secret":"integration-high-entropy-login-secret"}`))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("login status=%d", response.StatusCode)
	}
	if response.TLS == nil || response.TLS.Version != tls.VersionTLS13 {
		cancel()
		t.Fatalf("login TLS=%v want TLS1.3", response.TLS)
	}
	var issued sessionLoginResponse
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil {
		cancel()
		t.Fatal(err)
	}
	if issued.SessionCredential == "" {
		cancel()
		t.Fatal("login did not return session credential")
	}
	grant, err := runtime.provider.Resolve(context.Background(), []byte(issued.SessionCredential))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if grant.Identity.ID != "integration-character" || !grant.AllowActiveTakeover || grant.RevocationScope == "" {
		cancel()
		t.Fatalf("issued grant=%+v", grant)
	}
	scopeMu.Lock()
	activeScopeCount := len(scopes)
	scopeMu.Unlock()
	if activeScopeCount != 1 {
		cancel()
		t.Fatalf("active scopes=%d want=1", activeScopeCount)
	}

	logoutRequest, err := http.NewRequest(http.MethodPost, "https://"+runtime.Addr().String()+"/v1/session/logout", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	logoutRequest.Header.Set("Authorization", "Bearer "+issued.SessionCredential)
	logoutResponse, err := client.Do(logoutRequest)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_ = logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusNoContent {
		cancel()
		t.Fatalf("logout status=%d", logoutResponse.StatusCode)
	}
	if _, err := runtime.provider.Resolve(context.Background(), []byte(issued.SessionCredential)); err == nil {
		cancel()
		t.Fatal("logout did not revoke issued credential")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("issued session runtime did not stop")
	}
}

func TestIssuedLoginModeConflictsWithStaticTrustedCredentialFile(t *testing.T) {
	accountFile := writeSessionLoginAccountFile(t, "integration-conflict", "integration", "secret", "integration-character", false)
	certFile, keyFile, _ := writeTrustedTLSCertificate(t)
	configureSessionLoginFlags(t, accountFile, certFile, keyFile)

	_, runtime, _, err := loadRuntimeTrustedCharacterAuthenticator("config/static-auth.json", "127.0.0.1:7777")
	if runtime != nil {
		t.Fatal("conflicting auth mode must not return runtime")
	}
	if err != errSessionLoginAuthModeConflict {
		t.Fatalf("err=%v want=%v", err, errSessionLoginAuthModeConflict)
	}
}

func writeSessionLoginAccountFile(t *testing.T, revision, loginID, secret, characterID string, takeover bool) string {
	t.Helper()
	digest := sha256.Sum256([]byte(secret))
	definition := sessionLoginDefinition{
		SchemaVersion: sessionLoginSchemaVersion,
		Revision:      revision,
		Accounts: []sessionLoginAccount{{
			LoginID:             loginID,
			LoginSecretSHA256:   hex.EncodeToString(digest[:]),
			CharacterID:         characterID,
			AllowActiveTakeover: takeover,
		}},
	}
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session-login.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func configureSessionLoginFlags(t *testing.T, accountFile, certFile, keyFile string) {
	t.Helper()
	oldAccountFile := *sessionLoginAccountFile
	oldListen := *sessionLoginTLSListen
	oldCert := *sessionLoginTLSCertFile
	oldKey := *sessionLoginTLSKeyFile
	oldTTL := *issuedSessionCredentialTTL
	t.Cleanup(func() {
		*sessionLoginAccountFile = oldAccountFile
		*sessionLoginTLSListen = oldListen
		*sessionLoginTLSCertFile = oldCert
		*sessionLoginTLSKeyFile = oldKey
		*issuedSessionCredentialTTL = oldTTL
	})
	*sessionLoginAccountFile = accountFile
	*sessionLoginTLSListen = "127.0.0.1:0"
	*sessionLoginTLSCertFile = certFile
	*sessionLoginTLSKeyFile = keyFile
	*issuedSessionCredentialTTL = 5 * time.Minute
}
