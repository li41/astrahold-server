package tcpudp

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestTrustedCharacterConnectionAuthenticatorConsumesPrefaceAndBypassesLegacyIdentityFactory(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	cfg.TrustedCharacterAuthenticationTimeout = time.Second
	identity, err := characteridentity.NewTrusted("character:authenticated-preface")
	if err != nil {
		t.Fatal(err)
	}
	var legacyFactoryCalls atomic.Int32
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) {
		legacyFactoryCalls.Add(1)
		return characteridentity.NewEphemeral()
	}
	requests := make(chan TrustedCharacterConnectionAuthenticationRequest, 1)
	cfg.TrustedCharacterConnectionAuthenticator = func(ctx context.Context, request TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		if _, ok := ctx.Deadline(); !ok {
			return TrustedCharacterConnectionAuthentication{}, errors.New("authentication context has no deadline")
		}
		proof := make([]byte, 5)
		if _, err := io.ReadFull(request.Connection, proof); err != nil {
			return TrustedCharacterConnectionAuthentication{}, err
		}
		if string(proof) != "proof" {
			return TrustedCharacterConnectionAuthentication{}, errors.New("invalid proof")
		}
		requests <- request
		return TrustedCharacterConnectionAuthentication{Identity: identity, TakeoverAuthorizer: allowCharacterTakeoverForTest}, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	welcome := readSessionWelcome(t, conn, codec)
	if legacyFactoryCalls.Load() != 0 {
		t.Fatalf("legacy identity factory calls=%d want=0", legacyFactoryCalls.Load())
	}
	select {
	case request := <-requests:
		if uint64(request.CandidateSessionID) != welcome.SessionID {
			t.Fatalf("auth session=%d welcome=%d", request.CandidateSessionID, welcome.SessionID)
		}
		if request.AllocatedEntityID != welcome.EntityID {
			t.Fatalf("auth allocated entity=%d welcome=%d", request.AllocatedEntityID, welcome.EntityID)
		}
		if request.RemoteAddress == "" || request.Connection == nil {
			t.Fatalf("invalid auth request=%#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("authenticator request not observed")
	}
}

func TestTrustedCharacterConnectionAuthenticatorRejectsInvalidResultBeforeWelcome(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	cfg.TrustedCharacterConnectionAuthenticator = func(context.Context, TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		ephemeral, err := characteridentity.NewEphemeral()
		if err != nil {
			return TrustedCharacterConnectionAuthentication{}, err
		}
		return TrustedCharacterConnectionAuthentication{Identity: ephemeral}, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(conn, codec); err == nil {
		t.Fatal("invalid authenticated result received SessionWelcome")
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_connection_authenticate" || !errors.Is(event.Err, ErrInvalidTrustedCharacterAuthenticationResult) {
			t.Fatalf("authentication rejection event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing invalid authentication result event")
	}
}

func TestTrustedCharacterConnectionAuthenticationTimeoutBoundsBlockingIO(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	cfg.TrustedCharacterAuthenticationTimeout = 50 * time.Millisecond
	cfg.TrustedCharacterConnectionAuthenticator = func(_ context.Context, request TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		var one [1]byte
		_, err := request.Connection.Read(one[:])
		return TrustedCharacterConnectionAuthentication{}, err
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(conn, codec); err == nil {
		t.Fatal("timed-out authentication received SessionWelcome")
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_connection_authenticate" || !errors.Is(event.Err, ErrTrustedCharacterAuthenticationFailed) {
			t.Fatalf("authentication timeout event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing authentication timeout event")
	}
}

func TestAuthenticatedTakeoverUsesConnectionScopedAuthorizerInsteadOfGlobalFallback(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:authenticated-takeover")
	if err != nil {
		t.Fatal(err)
	}
	var authCalls atomic.Int32
	var scopedAuthorizerCalls atomic.Int32
	var globalAuthorizerCalls atomic.Int32
	cfg.CharacterTakeoverAuthorizer = func(context.Context, CharacterTakeoverRequest) error {
		globalAuthorizerCalls.Add(1)
		return errors.New("global authorizer must not be used")
	}
	cfg.TrustedCharacterConnectionAuthenticator = func(_ context.Context, request TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		proof := make([]byte, 5)
		if _, err := io.ReadFull(request.Connection, proof); err != nil {
			return TrustedCharacterConnectionAuthentication{}, err
		}
		if string(proof) != "proof" {
			return TrustedCharacterConnectionAuthentication{}, errors.New("invalid proof")
		}
		authCalls.Add(1)
		return TrustedCharacterConnectionAuthentication{
			Identity: identity,
			TakeoverAuthorizer: func(_ context.Context, request CharacterTakeoverRequest) error {
				scopedAuthorizerCalls.Add(1)
				if request.Identity != identity {
					return errors.New("authorizer identity mismatch")
				}
				return nil
			},
		}, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	if _, err := oldConn.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	oldWelcome := readSessionWelcome(t, oldConn, codec)

	newConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer newConn.Close()
	if _, err := newConn.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	newWelcome := readSessionWelcome(t, newConn, codec)
	if newWelcome.EntityID != oldWelcome.EntityID || newWelcome.SessionID == oldWelcome.SessionID {
		t.Fatalf("unexpected authenticated takeover old=%#v new=%#v", oldWelcome, newWelcome)
	}
	if authCalls.Load() != 2 {
		t.Fatalf("authenticator calls=%d want=2", authCalls.Load())
	}
	if scopedAuthorizerCalls.Load() != 1 {
		t.Fatalf("connection-scoped authorizer calls=%d want=1", scopedAuthorizerCalls.Load())
	}
	if globalAuthorizerCalls.Load() != 0 {
		t.Fatalf("global authorizer calls=%d want=0", globalAuthorizerCalls.Load())
	}
	select {
	case transfer := <-runtime.transfers:
		if uint64(transfer.expected.SessionID) != oldWelcome.SessionID || uint64(transfer.result.SessionID) != newWelcome.SessionID {
			t.Fatalf("authenticated transfer=%#v", transfer)
		}
	case <-time.After(time.Second):
		t.Fatal("authenticated takeover did not reach F.19 transfer")
	}
}

func TestAuthenticatedTakeoverWithoutScopedAuthorizerDoesNotFallbackToGlobal(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:authenticated-no-fallback")
	if err != nil {
		t.Fatal(err)
	}
	var globalAuthorizerCalls atomic.Int32
	cfg.CharacterTakeoverAuthorizer = func(context.Context, CharacterTakeoverRequest) error {
		globalAuthorizerCalls.Add(1)
		return nil
	}
	cfg.TrustedCharacterConnectionAuthenticator = func(_ context.Context, request TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		proof := make([]byte, 5)
		if _, err := io.ReadFull(request.Connection, proof); err != nil {
			return TrustedCharacterConnectionAuthentication{}, err
		}
		return TrustedCharacterConnectionAuthentication{Identity: identity}, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	if _, err := oldConn.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	oldWelcome := readSessionWelcome(t, oldConn, codec)

	candidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if _, err := candidate.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	_ = candidate.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(candidate, codec); err == nil {
		t.Fatal("authenticated takeover without scoped authorizer received SessionWelcome")
	}
	if globalAuthorizerCalls.Load() != 0 {
		t.Fatalf("authenticated path fell back to global authorizer calls=%d", globalAuthorizerCalls.Load())
	}
	select {
	case transfer := <-runtime.transfers:
		t.Fatalf("unauthorized authenticated candidate reached transfer=%#v", transfer)
	default:
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_takeover_authorize" || !errors.Is(event.Err, ErrCharacterTakeoverUnauthorized) {
			t.Fatalf("authenticated no-fallback event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing authenticated no-fallback rejection event")
	}

	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(oldConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 53, Message: action}, codec); err != nil {
		t.Fatalf("old owner was closed by unauthorized authenticated candidate: %v", err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != oldWelcome.SessionID || call.seq != 53 || call.action != action {
			t.Fatalf("old owner route=%#v welcome=%#v", call, oldWelcome)
		}
	case <-time.After(time.Second):
		t.Fatal("old owner stopped routing after authenticated no-fallback rejection")
	}
}

func TestSuccessfulAuthenticationResetsTransportDeadlineBeforeGameBootstrap(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	cfg.TrustedCharacterAuthenticationTimeout = 40 * time.Millisecond
	identity, err := characteridentity.NewTrusted("character:authenticated-deadline-reset")
	if err != nil {
		t.Fatal(err)
	}
	cfg.TrustedCharacterConnectionAuthenticator = func(_ context.Context, request TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error) {
		var proof [1]byte
		if _, err := io.ReadFull(request.Connection, proof[:]); err != nil {
			return TrustedCharacterConnectionAuthentication{}, err
		}
		return TrustedCharacterConnectionAuthentication{Identity: identity}, nil
	}
	baseFactory := defaultPlayerFactory
	cfg.PlayerFactory = func(id session.ID, entityID world.EntityID) PlayerSpec {
		time.Sleep(70 * time.Millisecond)
		return baseFactory(id, entityID)
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(conn, codec); err != nil {
		t.Fatalf("post-auth GameV1 bootstrap inherited expired authentication deadline: %v", err)
	}
}

var _ = worldruntime.SessionOwnershipFence{}
