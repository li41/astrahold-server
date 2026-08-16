package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArgon2idSessionLoginRuntimeServesTLS13AndIssuesGameCredential(t *testing.T) {
	definition := sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "password-integration-001",
		Accounts: []sessionPasswordLoginAccount{{
			LoginID:             "human",
			PasswordArgon2ID:    testArgon2idPHC("correct horse battery staple", []byte("0123456789abcdef"), 64*1024, 3, 4),
			CharacterID:         "human-character",
			AllowActiveTakeover: true,
		}},
	}
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	accountFile := filepath.Join(t.TempDir(), "password-accounts.json")
	if err := os.WriteFile(accountFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	certFile, keyFile, roots := writeTrustedTLSCertificate(t)
	configureSessionLoginFlags(t, accountFile, certFile, keyFile)

	runtime, err := loadSessionLoginRuntime("127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.accountAuth.Method() != "argon2id-password" || runtime.accountAuth.Revision() != definition.Revision {
		t.Fatalf("account auth method=%q revision=%q", runtime.accountAuth.Method(), runtime.accountAuth.Revision())
	}

	replaceScopes := func([]string) int { return 0 }
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
	response, err := client.Post(loginURL, "application/json", strings.NewReader(`{"login_id":"human","login_secret":"correct horse battery staple"}`))
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
	grant, err := runtime.provider.Resolve(context.Background(), []byte(issued.SessionCredential))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if grant.Identity.ID != "human-character" || !grant.AllowActiveTakeover || grant.RevocationScope == "" {
		cancel()
		t.Fatalf("issued grant=%+v", grant)
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
