package tcpudp

import (
	"context"
	"errors"
	"net"
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

func TestCharacterTakeoverAuthorizationDefaultsDeny(t *testing.T) {
	server := NewServer(DefaultConfig(), newTakeoverRuntime(), gamev1.Codec{})
	identity, err := characteridentity.NewTrusted("character:takeover-auth-default-deny")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{
		SessionID:    7,
		EntityID:     9,
		CharacterID: identity.ID,
		Epoch:        3,
	}
	if err := server.authorizeCharacterTakeover(context.Background(), 11, identity, expected, "127.0.0.1:12345"); !errors.Is(err, ErrCharacterTakeoverUnauthorized) {
		t.Fatalf("authorize error=%v want ErrCharacterTakeoverUnauthorized", err)
	}
}

func TestCharacterTakeoverAuthorizationPassesExactExpectedFence(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:takeover-auth-context")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{
		SessionID:    17,
		EntityID:     19,
		CharacterID: identity.ID,
		Epoch:        23,
	}
	captured := make(chan CharacterTakeoverRequest, 1)
	cfg := DefaultConfig()
	cfg.CharacterTakeoverAuthorizer = func(_ context.Context, request CharacterTakeoverRequest) error {
		captured <- request
		return nil
	}
	server := NewServer(cfg, newTakeoverRuntime(), gamev1.Codec{})
	if err := server.authorizeCharacterTakeover(context.Background(), 29, identity, expected, "198.51.100.7:4444"); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-captured:
		if request.CandidateSessionID != 29 {
			t.Fatalf("candidate session=%d want=29", request.CandidateSessionID)
		}
		if request.Identity != identity {
			t.Fatalf("identity=%#v want=%#v", request.Identity, identity)
		}
		if request.ExpectedOwnership != expected {
			t.Fatalf("expected ownership=%#v want=%#v", request.ExpectedOwnership, expected)
		}
		if request.RemoteAddress != "198.51.100.7:4444" {
			t.Fatalf("remote address=%q", request.RemoteAddress)
		}
	default:
		t.Fatal("authorizer did not receive request")
	}
}

func TestTrustedActiveTakeoverWithoutAuthorizerKeepsOldPeerActive(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:takeover-auth-missing")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	oldWelcome := readSessionWelcome(t, oldConn, codec)

	candidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	_ = candidate.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(candidate, codec); err == nil {
		t.Fatal("unauthorized takeover received SessionWelcome")
	}
	select {
	case transfer := <-runtime.transfers:
		t.Fatalf("unauthorized takeover reached transfer=%#v", transfer)
	default:
	}

	select {
	case event := <-server.Errors():
		if event.Operation != "character_takeover_authorize" {
			t.Fatalf("network operation=%q err=%v", event.Operation, event.Err)
		}
		if !errors.Is(event.Err, ErrCharacterTakeoverUnauthorized) {
			t.Fatalf("network error=%v want unauthorized", event.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing takeover authorization rejection event")
	}

	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(oldConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 31, Message: action}, codec); err != nil {
		t.Fatalf("old peer was closed by unauthorized candidate: %v", err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != oldWelcome.SessionID || call.seq != 31 || call.action != action {
			t.Fatalf("old owner action route=%#v welcome=%#v", call, oldWelcome)
		}
	case <-time.After(time.Second):
		t.Fatal("old owner stopped routing after unauthorized candidate")
	}
}

func TestTrustedActiveTakeoverAuthorizerDenialPreservesCauseAndOldOwner(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:takeover-auth-denied")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	denied := errors.New("account takeover policy denied")
	requests := make(chan CharacterTakeoverRequest, 1)
	cfg.CharacterTakeoverAuthorizer = func(_ context.Context, request CharacterTakeoverRequest) error {
		requests <- request
		return denied
	}
	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	oldWelcome := readSessionWelcome(t, oldConn, codec)

	candidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	_ = candidate.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(candidate, codec); err == nil {
		t.Fatal("denied takeover received SessionWelcome")
	}

	select {
	case request := <-requests:
		if request.Identity != identity {
			t.Fatalf("authorizer identity=%#v want=%#v", request.Identity, identity)
		}
		if uint64(request.ExpectedOwnership.SessionID) != oldWelcome.SessionID || request.ExpectedOwnership.EntityID != oldWelcome.EntityID {
			t.Fatalf("authorizer expected=%#v old welcome=%#v", request.ExpectedOwnership, oldWelcome)
		}
		if request.CandidateSessionID == request.ExpectedOwnership.SessionID {
			t.Fatalf("candidate reused old session=%d", request.CandidateSessionID)
		}
		if request.RemoteAddress == "" {
			t.Fatal("authorizer remote address is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("authorizer was not called for active takeover")
	}
	select {
	case transfer := <-runtime.transfers:
		t.Fatalf("denied takeover reached transfer=%#v", transfer)
	default:
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_takeover_authorize" {
			t.Fatalf("network operation=%q err=%v", event.Operation, event.Err)
		}
		if !errors.Is(event.Err, ErrCharacterTakeoverUnauthorized) || !errors.Is(event.Err, denied) {
			t.Fatalf("network error=%v missing rejection classification/cause", event.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing authorizer denial event")
	}

	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(oldConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 37, Message: action}, codec); err != nil {
		t.Fatalf("old peer was closed by denied candidate: %v", err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != oldWelcome.SessionID || call.seq != 37 || call.action != action {
			t.Fatalf("old owner action route=%#v welcome=%#v", call, oldWelcome)
		}
	case <-time.After(time.Second):
		t.Fatal("old owner stopped routing after denied candidate")
	}
}
