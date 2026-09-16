package main

import (
	"context"
	"encoding/binary"
	"net"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/sessioncredential"
	"github.com/li41/astrahold-server/internal/world"
)

type authenticationSubjectTestProvider struct {
	grant sessioncredential.Grant
}

func (p authenticationSubjectTestProvider) Resolve(context.Context, []byte) (sessioncredential.Grant, error) {
	return p.grant, nil
}

func TestTrustedCharacterAuthenticatorPreservesAuthenticationSubject(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:account-one")
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := newTrustedCharacterAuthenticatorWithProvider(authenticationSubjectTestProvider{grant: sessioncredential.Grant{
		Identity:                 identity,
		AuthenticationSubject:    "1",
		AuthenticationGeneration: "generation-1",
	}})
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	credential := []byte("opaque-account-one-credential")
	go func() {
		header := make([]byte, trustedCharacterAuthHeaderBytes)
		copy(header, []byte(trustedCharacterAuthMagic))
		binary.BigEndian.PutUint16(header[len(trustedCharacterAuthMagic):], uint16(len(credential)))
		_, _ = clientConn.Write(append(header, credential...))
	}()

	result, err := authenticator.Authenticate(context.Background(), tcpudp.TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: session.ID(1),
		AllocatedEntityID:  world.EntityID(1),
		Connection:         serverConn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity != identity || result.AuthenticationSubject != "1" {
		t.Fatalf("authentication result=%#v", result)
	}
}
