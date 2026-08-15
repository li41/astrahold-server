package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/sessioncredential"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLoadTrustedCharacterAuthenticatorDisabledByDefault(t *testing.T) {
	authenticator, revision, err := loadTrustedCharacterAuthenticator("", "0.0.0.0:7777")
	if err != nil {
		t.Fatal(err)
	}
	if authenticator != nil || revision != "" {
		t.Fatalf("empty config must preserve ephemeral default, authenticator=%v revision=%q", authenticator != nil, revision)
	}
}

func TestTrustedCharacterAuthenticatorConsumesOnlyPrefaceAndReturnsServerOwnedIdentity(t *testing.T) {
	token := []byte("e3-keeper-secret")
	path := writeTrustedAuthConfig(t, token, "e3-keeper")
	authenticator, revision, err := loadTrustedCharacterAuthenticator(path, "127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	if revision != "s4e3-test-001" {
		t.Fatalf("revision=%q", revision)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	writeDone := make(chan error, 1)
	go func() {
		payload := append(encodeTrustedAuthPrefaceForTest(token), []byte("GAME")...)
		_, err := clientConn.Write(payload)
		writeDone <- err
	}()

	result, err := authenticator(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: session.ID(1),
		AllocatedEntityID:  world.EntityID(1),
		RemoteAddress:      "127.0.0.1:50000",
		Connection:         serverConn,
	})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := characteridentity.NewTrusted("e3-keeper")
	if result.Identity != want || result.TakeoverAuthorizer != nil {
		t.Fatalf("authentication=%+v want identity=%+v and nil takeover authorizer", result, want)
	}

	extra := make([]byte, 4)
	if _, err := serverConn.Read(extra); err != nil {
		t.Fatal(err)
	}
	if string(extra) != "GAME" {
		t.Fatalf("authenticator consumed post-preface bytes: %q", extra)
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client writer did not complete")
	}
}

func TestTrustedCharacterAuthenticatorDelegatesOpaqueCredentialToProvider(t *testing.T) {
	token := []byte("provider-owned-opaque-credential")
	binding, err := characteridentity.NewTrusted("provider-character")
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingSessionCredentialProvider{
		grant: sessioncredential.Grant{Identity: binding, AllowActiveTakeover: true},
	}
	authenticator, err := newTrustedCharacterAuthenticatorWithProvider(provider)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	writeDone := make(chan error, 1)
	go func() {
		payload := append(encodeTrustedAuthPrefaceForTest(token), []byte("GAME")...)
		_, err := clientConn.Write(payload)
		writeDone <- err
	}()

	result, err := authenticator.Authenticate(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: 1,
		AllocatedEntityID:  1,
		RemoteAddress:      "127.0.0.1:50000",
		Connection:         serverConn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity != binding {
		t.Fatalf("identity=%+v want=%+v", result.Identity, binding)
	}
	if result.TakeoverAuthorizer == nil {
		t.Fatal("provider takeover claim must become a connection-scoped authorizer")
	}
	if string(provider.credential) != string(token) {
		t.Fatalf("provider credential=%q want=%q", provider.credential, token)
	}

	extra := make([]byte, 4)
	if _, err := serverConn.Read(extra); err != nil {
		t.Fatal(err)
	}
	if string(extra) != "GAME" {
		t.Fatalf("provider-backed authenticator consumed post-preface bytes: %q", extra)
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client writer did not complete")
	}
}

func TestTrustedCharacterAuthenticatorRejectsProviderErrorAndInvalidGrant(t *testing.T) {
	providerErr := errors.New("credential backend unavailable")
	cases := []struct {
		name     string
		provider *recordingSessionCredentialProvider
		wantErr  error
	}{
		{
			name:     "provider error",
			provider: &recordingSessionCredentialProvider{err: providerErr},
			wantErr:  providerErr,
		},
		{
			name:     "invalid grant",
			provider: &recordingSessionCredentialProvider{},
			wantErr:  errTrustedCharacterCredentialProviderGrant,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authenticator, err := newTrustedCharacterAuthenticatorWithProvider(tc.provider)
			if err != nil {
				t.Fatal(err)
			}
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()
			defer clientConn.Close()
			go func() { _, _ = clientConn.Write(encodeTrustedAuthPrefaceForTest([]byte("opaque"))) }()

			_, err = authenticator.Authenticate(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
				CandidateSessionID: 1, AllocatedEntityID: 1, Connection: serverConn,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want errors.Is(_, %v)", err, tc.wantErr)
			}
		})
	}
}

func TestTrustedCharacterAuthenticatorRejectsUnknownCredential(t *testing.T) {
	path := writeTrustedAuthConfig(t, []byte("known-secret"), "e3-keeper")
	authenticator, _, err := loadTrustedCharacterAuthenticator(path, "[::1]:7777")
	if err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	go func() { _, _ = clientConn.Write(encodeTrustedAuthPrefaceForTest([]byte("wrong-secret"))) }()
	if _, err := authenticator(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: 1, AllocatedEntityID: 1, Connection: serverConn,
	}); err == nil {
		t.Fatal("unknown credential must fail closed")
	}
}

func TestTrustedCharacterAuthRequiresLoopbackWhenEnabled(t *testing.T) {
	path := writeTrustedAuthConfig(t, []byte("secret"), "e3-keeper")
	for _, address := range []string{"0.0.0.0:7777", "192.0.2.1:7777", "localhost:7777", "bad"} {
		if _, _, err := loadTrustedCharacterAuthenticator(path, address); err == nil {
			t.Fatalf("expected address %q to be rejected", address)
		}
	}
}

func TestTrustedCharacterAuthConfigIsStrictAndRejectsDuplicateDigest(t *testing.T) {
	digest := sha256.Sum256([]byte("secret"))
	hexDigest := hex.EncodeToString(digest[:])
	cases := []string{
		fmt.Sprintf(`{"schema_version":1,"revision":"x","credentials":[{"token_sha256":"%s","character_id":"a"}],"unknown":true}`, hexDigest),
		fmt.Sprintf(`{"schema_version":1,"revision":"x","credentials":[{"token_sha256":"%s","character_id":"a"},{"token_sha256":"%s","character_id":"b"}]}`, hexDigest, hexDigest),
	}
	for index, content := range cases {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("auth-%d.json", index))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadTrustedCharacterAuthenticator(path, "127.0.0.1:7777"); err == nil {
			t.Fatalf("case %d should fail", index)
		}
	}
}

func TestTrustedCharacterAuthSchemaV1RejectsLifecycleFields(t *testing.T) {
	digest := sha256.Sum256([]byte("legacy-secret"))
	_, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthLegacySchemaVersion,
		Revision:      "legacy-with-lifecycle",
		Credentials: []trustedCharacterAuthCredential{{
			TokenSHA256: hex.EncodeToString(digest[:]),
			CharacterID: "legacy-character",
			ExpiresAt:   "2026-08-17T00:00:00Z",
		}},
	})
	if err == nil {
		t.Fatal("schema v1 must not silently accept lifecycle fields")
	}
}

func TestStaticTrustedCharacterCredentialProviderLifecycleBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	activeToken := []byte("active-secret")
	notYetToken := []byte("not-yet-secret")
	expiredToken := []byte("expired-secret")
	revokedToken := []byte("revoked-secret")

	provider, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "s4f2-lifecycle-boundaries",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("active", activeToken, "lifecycle-character", now, now.Add(time.Hour), time.Time{}),
			lifecycleCredential("not-yet", notYetToken, "lifecycle-character", now.Add(time.Second), now.Add(time.Hour), time.Time{}),
			lifecycleCredential("expired", expiredToken, "lifecycle-character", time.Time{}, now, time.Time{}),
			lifecycleCredential("revoked", revokedToken, "lifecycle-character", time.Time{}, now.Add(time.Hour), now),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.now = func() time.Time { return now }

	if _, err := provider.Resolve(context.Background(), activeToken); err != nil {
		t.Fatalf("active credential rejected: %v", err)
	}
	for _, tc := range []struct {
		name    string
		token   []byte
		wantErr error
	}{
		{name: "not yet", token: notYetToken, wantErr: sessioncredential.ErrCredentialNotYetValid},
		{name: "expired", token: expiredToken, wantErr: sessioncredential.ErrCredentialExpired},
		{name: "revoked", token: revokedToken, wantErr: sessioncredential.ErrCredentialRevoked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := provider.Resolve(context.Background(), tc.token)
			if !errors.Is(err, errTrustedCharacterAuthCredential) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want credential rejection plus %v", err, tc.wantErr)
			}
		})
	}
}

func TestStaticTrustedCharacterCredentialProviderSupportsRotationOverlap(t *testing.T) {
	overlap := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	oldToken := []byte("rotation-old")
	newToken := []byte("rotation-new")
	provider, err := newStaticTrustedCharacterCredentialProvider(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthSchemaVersion,
		Revision:      "s4f2-rotation-overlap",
		Credentials: []trustedCharacterAuthCredential{
			lifecycleCredential("keeper-old", oldToken, "rotation-character", overlap.Add(-time.Hour), overlap.Add(10*time.Minute), time.Time{}),
			lifecycleCredential("keeper-new", newToken, "rotation-character", overlap.Add(-10*time.Minute), overlap.Add(time.Hour), time.Time{}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.now = func() time.Time { return overlap }

	oldGrant, err := provider.Resolve(context.Background(), oldToken)
	if err != nil {
		t.Fatalf("old credential must remain valid during overlap: %v", err)
	}
	newGrant, err := provider.Resolve(context.Background(), newToken)
	if err != nil {
		t.Fatalf("new credential must be valid during overlap: %v", err)
	}
	if oldGrant.Identity != newGrant.Identity {
		t.Fatalf("rotation credentials resolved different identities: old=%+v new=%+v", oldGrant.Identity, newGrant.Identity)
	}

	provider.now = func() time.Time { return overlap.Add(11 * time.Minute) }
	if _, err := provider.Resolve(context.Background(), oldToken); !errors.Is(err, sessioncredential.ErrCredentialExpired) {
		t.Fatalf("old credential err=%v want expired after overlap", err)
	}
	if _, err := provider.Resolve(context.Background(), newToken); err != nil {
		t.Fatalf("new credential must survive old credential expiry: %v", err)
	}
}

func TestTrustedCharacterAuthSchemaV2RejectsDuplicateCredentialIDAndInvalidWindow(t *testing.T) {
	first := []byte("first-secret")
	second := []byte("second-secret")
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	cases := []trustedCharacterAuthDefinition{
		{
			SchemaVersion: trustedCharacterAuthSchemaVersion,
			Revision:      "duplicate-id",
			Credentials: []trustedCharacterAuthCredential{
				lifecycleCredential("same-id", first, "a", time.Time{}, time.Time{}, time.Time{}),
				lifecycleCredential("same-id", second, "b", time.Time{}, time.Time{}, time.Time{}),
			},
		},
		{
			SchemaVersion: trustedCharacterAuthSchemaVersion,
			Revision:      "invalid-window",
			Credentials: []trustedCharacterAuthCredential{
				lifecycleCredential("bad-window", first, "a", now, now, time.Time{}),
			},
		},
	}
	for index, definition := range cases {
		if _, err := newStaticTrustedCharacterCredentialProvider(definition); err == nil {
			t.Fatalf("case %d must fail strict lifecycle config validation", index)
		}
	}
}

type recordingSessionCredentialProvider struct {
	credential []byte
	grant      sessioncredential.Grant
	err        error
}

func (p *recordingSessionCredentialProvider) Resolve(_ context.Context, credential []byte) (sessioncredential.Grant, error) {
	p.credential = append(p.credential[:0], credential...)
	return p.grant, p.err
}

func writeTrustedAuthConfig(t *testing.T, token []byte, characterID string) string {
	t.Helper()
	digest := sha256.Sum256(token)
	content := fmt.Sprintf(`{"schema_version":1,"revision":"s4e3-test-001","credentials":[{"token_sha256":"%s","character_id":"%s"}]}`, hex.EncodeToString(digest[:]), characterID)
	path := filepath.Join(t.TempDir(), "trusted-auth.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func lifecycleCredential(credentialID string, token []byte, characterID string, notBefore, expiresAt, revokedAt time.Time) trustedCharacterAuthCredential {
	return trustedCharacterAuthCredential{
		CredentialID: credentialID,
		TokenSHA256:  credentialDigestHex(token),
		CharacterID:  characterID,
		NotBefore:    optionalRFC3339(notBefore),
		ExpiresAt:    optionalRFC3339(expiresAt),
		RevokedAt:    optionalRFC3339(revokedAt),
	}
}

func credentialDigestHex(token []byte) string {
	digest := sha256.Sum256(token)
	return hex.EncodeToString(digest[:])
}

func optionalRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func encodeTrustedAuthPrefaceForTest(token []byte) []byte {
	payload := make([]byte, trustedCharacterAuthHeaderBytes+len(token))
	copy(payload, trustedCharacterAuthMagic)
	binary.BigEndian.PutUint16(payload[len(trustedCharacterAuthMagic):trustedCharacterAuthHeaderBytes], uint16(len(token)))
	copy(payload[trustedCharacterAuthHeaderBytes:], token)
	return payload
}
