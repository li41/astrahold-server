package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestTrustedCharacterCredentialActiveTakeoverIsExplicitAndConnectionScoped(t *testing.T) {
	plainToken := []byte("plain-token")
	takeoverToken := []byte("takeover-token")
	plainDigest := sha256.Sum256(plainToken)
	takeoverDigest := sha256.Sum256(takeoverToken)
	authenticator, err := newTrustedCharacterAuthenticator(trustedCharacterAuthDefinition{
		SchemaVersion: trustedCharacterAuthLegacySchemaVersion,
		Revision:      "s4e4-test-001",
		Credentials: []trustedCharacterAuthCredential{
			{TokenSHA256: hex.EncodeToString(plainDigest[:]), CharacterID: "same-character"},
			{TokenSHA256: hex.EncodeToString(takeoverDigest[:]), CharacterID: "same-character", AllowActiveTakeover: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	plain := authenticateCredentialForTest(t, authenticator, plainToken)
	if plain.TakeoverAuthorizer != nil {
		t.Fatal("credential without allow_active_takeover must fail closed for active takeover")
	}

	allowed := authenticateCredentialForTest(t, authenticator, takeoverToken)
	if allowed.TakeoverAuthorizer == nil {
		t.Fatal("credential with allow_active_takeover must carry a connection-scoped authorizer")
	}
	request := tcpudp.CharacterTakeoverRequest{
		CandidateSessionID: session.ID(2),
		Identity:           allowed.Identity,
		ExpectedOwnership: worldruntime.SessionOwnershipFence{
			SessionID:   session.ID(1),
			EntityID:    world.EntityID(7),
			CharacterID: allowed.Identity.ID,
			Epoch:       1,
		},
		RemoteAddress: "127.0.0.1:50001",
	}
	if err := allowed.TakeoverAuthorizer(context.Background(), request); err != nil {
		t.Fatalf("expected authorized exact-character takeover: %v", err)
	}

	otherIdentity, err := characteridentity.NewTrusted("other-character")
	if err != nil {
		t.Fatal(err)
	}
	request.Identity = otherIdentity
	request.ExpectedOwnership.CharacterID = otherIdentity.ID
	if err := allowed.TakeoverAuthorizer(context.Background(), request); err == nil {
		t.Fatal("connection-scoped takeover credential must not authorize another character")
	}
}

func authenticateCredentialForTest(t *testing.T, authenticator *trustedCharacterAuthenticator, credential []byte) tcpudp.TrustedCharacterConnectionAuthentication {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		preface := make([]byte, trustedCharacterAuthHeaderBytes+len(credential))
		copy(preface, []byte(trustedCharacterAuthMagic))
		binary.BigEndian.PutUint16(preface[len(trustedCharacterAuthMagic):], uint16(len(credential)))
		copy(preface[trustedCharacterAuthHeaderBytes:], credential)
		_, _ = clientConn.Write(preface)
	}()

	result, err := authenticator.Authenticate(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: session.ID(9),
		AllocatedEntityID:  world.EntityID(9),
		RemoteAddress:      "127.0.0.1:50000",
		Connection:         serverConn,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
