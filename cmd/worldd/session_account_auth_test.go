package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func TestArgon2idSessionLoginAuthenticatorSelectsServerClaims(t *testing.T) {
	definition := sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "password-accounts-001",
		Accounts: []sessionPasswordLoginAccount{
			{
				LoginID:             "alice",
				PasswordArgon2ID:    testArgon2idPHC("correct horse battery staple", []byte("0123456789abcdef"), 64*1024, 3, 4),
				CharacterID:         "alice-character",
				AllowActiveTakeover: true,
			},
			{
				LoginID:          "bob",
				PasswordArgon2ID: testArgon2idPHC("another human password", []byte("fedcba9876543210"), 64*1024, 3, 4),
				CharacterID:      "bob-character",
			},
		},
	}
	authenticator, err := newArgon2idSessionLoginAuthenticator(definition)
	if err != nil {
		t.Fatal(err)
	}
	if authenticator.Revision() != definition.Revision || authenticator.Method() != "argon2id-password" {
		t.Fatalf("metadata revision=%q method=%q", authenticator.Revision(), authenticator.Method())
	}
	grant, err := authenticator.Authenticate(context.Background(), "alice", []byte("correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	if grant.Identity.ID != "alice-character" || !grant.AllowActiveTakeover || grant.RevocationScope != "" {
		t.Fatalf("grant=%+v", grant)
	}
}

func TestArgon2idSessionLoginAuthenticatorRunsKDFForUnknownLogin(t *testing.T) {
	authenticator, err := newArgon2idSessionLoginAuthenticator(sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "password-accounts-enumeration",
		Accounts: []sessionPasswordLoginAccount{{
			LoginID:          "alice",
			PasswordArgon2ID: testArgon2idPHC("known-password", []byte("0123456789abcdef"), 64*1024, 3, 4),
			CharacterID:      "alice-character",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := authenticator.derive
	var calls atomic.Int32
	authenticator.derive = func(password, salt []byte, timeCost, memory uint32, threads uint8, keyLen uint32) []byte {
		calls.Add(1)
		return original(password, salt, timeCost, memory, threads, keyLen)
	}
	if _, err := authenticator.Authenticate(context.Background(), "missing", []byte("known-password")); err == nil {
		t.Fatal("unknown login must remain rejected even when the password matches the dummy verifier")
	}
	if calls.Load() != 1 {
		t.Fatalf("unknown login KDF calls=%d want=1", calls.Load())
	}
	if _, err := authenticator.Authenticate(context.Background(), "alice", []byte("wrong-password")); err == nil {
		t.Fatal("wrong password must fail")
	}
	if calls.Load() != 2 {
		t.Fatalf("known wrong password KDF calls=%d want=2 total", calls.Load())
	}
}

func TestArgon2idSessionLoginAuthenticatorRejectsWeakAndMixedPolicies(t *testing.T) {
	cases := map[string]string{
		"low-memory": testArgon2idPHC("password", []byte("0123456789abcdef"), 32*1024, 3, 4),
		"low-time":   testArgon2idPHC("password", []byte("0123456789abcdef"), 64*1024, 2, 4),
	}
	for name, phc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newArgon2idSessionLoginAuthenticator(sessionPasswordLoginDefinition{
				SchemaVersion: sessionPasswordSchemaVersion,
				Revision:      name,
				Accounts: []sessionPasswordLoginAccount{{
					LoginID:          "alice",
					PasswordArgon2ID: phc,
					CharacterID:      "alice-character",
				}},
			})
			if err == nil {
				t.Fatal("weak Argon2id policy must fail closed")
			}
		})
	}

	_, err := newArgon2idSessionLoginAuthenticator(sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "mixed-policy",
		Accounts: []sessionPasswordLoginAccount{
			{LoginID: "alice", PasswordArgon2ID: testArgon2idPHC("a", []byte("0123456789abcdef"), 64*1024, 3, 4), CharacterID: "alice-character"},
			{LoginID: "bob", PasswordArgon2ID: testArgon2idPHC("b", []byte("fedcba9876543210"), 128*1024, 3, 4), CharacterID: "bob-character"},
		},
	})
	if err == nil {
		t.Fatal("mixed Argon2id cost policy must fail closed")
	}
}

func TestSessionAccountAuthenticatorFactorySupportsLegacyAndPasswordSchemas(t *testing.T) {
	legacy := writeSessionLoginAccountFile(t, "legacy-provider", "legacy", "high-entropy-secret", "legacy-character", false)
	legacyAuth, err := loadSessionAccountAuthenticator(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if legacyAuth.Method() != "sha256-high-entropy" || legacyAuth.Revision() != "legacy-provider" {
		t.Fatalf("legacy metadata method=%q revision=%q", legacyAuth.Method(), legacyAuth.Revision())
	}

	definition := sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "password-provider",
		Accounts: []sessionPasswordLoginAccount{{
			LoginID:          "human",
			PasswordArgon2ID: testArgon2idPHC("human password 2026", []byte("abcdefghijklmnop"), 64*1024, 3, 4),
			CharacterID:      "human-character",
		}},
	}
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "password-accounts.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	passwordAuth, err := loadSessionAccountAuthenticator(path)
	if err != nil {
		t.Fatal(err)
	}
	if passwordAuth.Method() != "argon2id-password" || passwordAuth.Revision() != "password-provider" {
		t.Fatalf("password metadata method=%q revision=%q", passwordAuth.Method(), passwordAuth.Revision())
	}
}

func TestArgon2idHTTPWrongAndUnknownCredentialsShareResponse(t *testing.T) {
	authenticator, err := newArgon2idSessionLoginAuthenticator(sessionPasswordLoginDefinition{
		SchemaVersion: sessionPasswordSchemaVersion,
		Revision:      "http-enumeration",
		Accounts: []sessionPasswordLoginAccount{{
			LoginID:          "alice",
			PasswordArgon2ID: testArgon2idPHC("right-password", []byte("0123456789abcdef"), 64*1024, 3, 4),
			CharacterID:      "alice-character",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC) })
	runtime.accountAuth, err = newSessionAccountAuthRuntime(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	runtime.replaceScopes = func([]string) int { return 0 }

	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		runtime.handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(body)))
		return recorder
	}
	wrong := request(`{"login_id":"alice","login_secret":"wrong-password"}`)
	unknown := request(`{"login_id":"missing","login_secret":"wrong-password"}`)
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("statuses wrong=%d unknown=%d", wrong.Code, unknown.Code)
	}
	if wrong.Body.String() != unknown.Body.String() || wrong.Body.String() != "{\"error\":\"invalid_credentials\"}\n" {
		t.Fatalf("credential error shapes differ: wrong=%q unknown=%q", wrong.Body.String(), unknown.Body.String())
	}
}

func testArgon2idPHC(password string, salt []byte, memory, timeCost uint32, threads uint8) string {
	digest := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, sessionPasswordArgon2DigestBytes)
	return "$argon2id$v=19$m=" + itoa(memory) + ",t=" + itoa(timeCost) + ",p=" + itoa(uint32(threads)) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(digest)
}

func itoa(value uint32) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buffer [10]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}
