package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
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

func encodeTrustedAuthPrefaceForTest(token []byte) []byte {
	payload := make([]byte, trustedCharacterAuthHeaderBytes+len(token))
	copy(payload, trustedCharacterAuthMagic)
	binary.BigEndian.PutUint16(payload[len(trustedCharacterAuthMagic):trustedCharacterAuthHeaderBytes], uint16(len(token)))
	copy(payload[trustedCharacterAuthHeaderBytes:], token)
	return payload
}
