package main

import (
	"context"
	"net"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

func TestTrustedCharacterAuthenticatorPropagatesServerOwnedRevocationScope(t *testing.T) {
	binding, err := characteridentity.NewTrusted("scope-propagation-character")
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingSessionCredentialProvider{
		grant: sessioncredential.Grant{
			Identity:        binding,
			RevocationScope: "static-v2:test-scope",
		},
	}
	authenticator, err := newTrustedCharacterAuthenticatorWithProvider(provider)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	go func() {
		_, _ = clientConn.Write(encodeTrustedAuthPrefaceForTest([]byte("scope-proof")))
	}()

	result, err := authenticator.Authenticate(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: 1,
		AllocatedEntityID:  1,
		Connection:         serverConn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity != binding {
		t.Fatalf("identity=%+v want=%+v", result.Identity, binding)
	}
	if result.RevocationScope != provider.grant.RevocationScope {
		t.Fatalf("revocation scope=%q want=%q", result.RevocationScope, provider.grant.RevocationScope)
	}
}
