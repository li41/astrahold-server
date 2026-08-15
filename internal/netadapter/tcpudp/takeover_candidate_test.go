package tcpudp

import (
	"context"
	"errors"
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

func TestTakeoverCandidateGateAllowsOnlyOneInflightCandidatePerCharacter(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:candidate-single")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 9, CharacterID: identity.ID, Epoch: 3}
	gate := newTakeoverCandidateGate(10*time.Second, 2*time.Second)
	now := time.Unix(100, 0)
	gate.now = func() time.Time { return now }

	first, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 11, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Valid() {
		t.Fatalf("invalid first lease=%#v", first)
	}
	if _, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 12, Identity: identity, ExpectedOwnership: expected}); !errors.Is(err, ErrCharacterTakeoverCandidateReserved) {
		t.Fatalf("second acquire error=%v want reserved", err)
	}

	otherIdentity, err := characteridentity.NewTrusted("character:candidate-other")
	if err != nil {
		t.Fatal(err)
	}
	otherExpected := worldruntime.SessionOwnershipFence{SessionID: 17, EntityID: 19, CharacterID: otherIdentity.ID, Epoch: 5}
	if _, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 21, Identity: otherIdentity, ExpectedOwnership: otherExpected}); err != nil {
		t.Fatalf("different character was globally blocked: %v", err)
	}
}

func TestTakeoverCandidateGateExpiryMintsNewGenerationAndStaleReleaseCannotClearReplacement(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:candidate-expiry")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 9, CharacterID: identity.ID, Epoch: 3}
	gate := newTakeoverCandidateGate(5*time.Second, 0)
	now := time.Unix(200, 0)
	gate.now = func() time.Time { return now }

	first, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 11, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Second)
	second, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 12, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation <= first.Generation {
		t.Fatalf("replacement generation=%d first=%d", second.Generation, first.Generation)
	}
	gate.release(first)
	if err := gate.validate(second); err != nil {
		t.Fatalf("stale release cleared replacement lease: %v", err)
	}
	if err := gate.validate(first); !errors.Is(err, ErrCharacterTakeoverCandidateStale) {
		t.Fatalf("old lease validate error=%v want stale", err)
	}
}

func TestTakeoverCandidateGateCommitInstallsExactOwnerCooldown(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:candidate-cooldown")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 9, CharacterID: identity.ID, Epoch: 3}
	gate := newTakeoverCandidateGate(10*time.Second, 2*time.Second)
	now := time.Unix(300, 0)
	gate.now = func() time.Time { return now }

	lease, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 11, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	newOwner := worldruntime.SessionOwnershipFence{SessionID: 11, EntityID: expected.EntityID, CharacterID: identity.ID, Epoch: 4}
	if err := gate.commit(lease, newOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 12, Identity: identity, ExpectedOwnership: newOwner}); !errors.Is(err, ErrCharacterTakeoverCoolingDown) {
		t.Fatalf("immediate acquire error=%v want cooldown", err)
	}

	now = now.Add(3 * time.Second)
	if _, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 13, Identity: identity, ExpectedOwnership: newOwner}); err != nil {
		t.Fatalf("expired cooldown still blocked current owner: %v", err)
	}
}

func TestTakeoverCandidateGateCooldownCannotBlockDifferentOwnershipFence(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:candidate-owner-change")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 9, CharacterID: identity.ID, Epoch: 3}
	gate := newTakeoverCandidateGate(10*time.Second, time.Minute)
	now := time.Unix(400, 0)
	gate.now = func() time.Time { return now }

	lease, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 11, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	cooldownOwner := worldruntime.SessionOwnershipFence{SessionID: 11, EntityID: 9, CharacterID: identity.ID, Epoch: 4}
	if err := gate.commit(lease, cooldownOwner); err != nil {
		t.Fatal(err)
	}

	newerOwner := worldruntime.SessionOwnershipFence{SessionID: 17, EntityID: 9, CharacterID: identity.ID, Epoch: 5}
	if _, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 19, Identity: identity, ExpectedOwnership: newerOwner}); err != nil {
		t.Fatalf("cooldown for older exact owner blocked newer ownership fence: %v", err)
	}
}

func TestTakeoverCandidateGateStaleCommitCannotOverwriteReplacementLease(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:candidate-stale-commit")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 9, CharacterID: identity.ID, Epoch: 3}
	gate := newTakeoverCandidateGate(5*time.Second, time.Minute)
	now := time.Unix(500, 0)
	gate.now = func() time.Time { return now }

	first, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 11, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Second)
	second, err := gate.acquire(CharacterTakeoverRequest{CandidateSessionID: 12, Identity: identity, ExpectedOwnership: expected})
	if err != nil {
		t.Fatal(err)
	}
	firstOwner := worldruntime.SessionOwnershipFence{SessionID: 11, EntityID: 9, CharacterID: identity.ID, Epoch: 4}
	if err := gate.commit(first, firstOwner); !errors.Is(err, ErrCharacterTakeoverCandidateStale) {
		t.Fatalf("stale commit error=%v want stale", err)
	}
	if err := gate.validate(second); err != nil {
		t.Fatalf("stale commit mutated replacement lease: %v", err)
	}
}

func TestTrustedActiveTakeoverCandidateLeaseSerializesAuthorization(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:candidate-serialize")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	entered := make(chan CharacterTakeoverRequest, 1)
	release := make(chan struct{})
	var authorizeCalls atomic.Int32
	cfg.CharacterTakeoverAuthorizer = func(_ context.Context, request CharacterTakeoverRequest) error {
		if authorizeCalls.Add(1) == 1 {
			entered <- request
			<-release
		}
		return nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	_ = readSessionWelcome(t, oldConn, codec)

	firstCandidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer firstCandidate.Close()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first candidate never entered authorizer")
	}

	secondCandidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer secondCandidate.Close()
	_ = secondCandidate.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(secondCandidate, codec); err == nil {
		t.Fatal("second in-flight candidate received SessionWelcome")
	}
	if authorizeCalls.Load() != 1 {
		t.Fatalf("authorizer calls=%d want=1 while first lease is active", authorizeCalls.Load())
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_takeover_candidate_acquire" || !errors.Is(event.Err, ErrCharacterTakeoverCandidateReserved) {
			t.Fatalf("candidate rejection event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing candidate reservation rejection event")
	}

	close(release)
	_ = readSessionWelcome(t, firstCandidate, codec)
}

func TestTrustedActiveTakeoverCooldownRejectsImmediateRetakeover(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	cfg.CharacterTakeoverCooldown = 30 * time.Second
	identity, err := characteridentity.NewTrusted("character:candidate-cooldown-integration")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	var authorizeCalls atomic.Int32
	cfg.CharacterTakeoverAuthorizer = func(context.Context, CharacterTakeoverRequest) error {
		authorizeCalls.Add(1)
		return nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	_ = readSessionWelcome(t, oldConn, codec)

	currentConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer currentConn.Close()
	currentWelcome := readSessionWelcome(t, currentConn, codec)
	if authorizeCalls.Load() != 1 {
		t.Fatalf("authorizer calls after first takeover=%d want=1", authorizeCalls.Load())
	}

	thirdConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer thirdConn.Close()
	_ = thirdConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(thirdConn, codec); err == nil {
		t.Fatal("cooldown candidate received SessionWelcome")
	}
	if authorizeCalls.Load() != 1 {
		t.Fatalf("cooldown candidate reached authorizer; calls=%d", authorizeCalls.Load())
	}
	select {
	case event := <-server.Errors():
		if event.Operation != "character_takeover_candidate_acquire" || !errors.Is(event.Err, ErrCharacterTakeoverCoolingDown) {
			t.Fatalf("cooldown rejection event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing cooldown rejection event")
	}

	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(currentConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 41, Message: action}, codec); err != nil {
		t.Fatalf("current owner was closed by cooldown candidate: %v", err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != currentWelcome.SessionID || call.seq != 41 || call.action != action {
			t.Fatalf("current owner action route=%#v welcome=%#v", call, currentWelcome)
		}
	case <-time.After(time.Second):
		t.Fatal("current owner stopped routing during cooldown")
	}
}

func TestTrustedActiveTakeoverTransferFailureReleasesCandidateLease(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:candidate-transfer-retry")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	cfg.CharacterTakeoverAuthorizer = allowCharacterTakeoverForTest

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	oldConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer oldConn.Close()
	_ = readSessionWelcome(t, oldConn, codec)

	runtime.mu.Lock()
	runtime.transferErr = errors.New("first transfer rejected")
	runtime.mu.Unlock()
	failedCandidate, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = failedCandidate.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(failedCandidate, codec); err == nil {
		t.Fatal("failed transfer candidate received SessionWelcome")
	}
	_ = failedCandidate.Close()
	select {
	case event := <-server.Errors():
		if event.Operation != "ownership_transfer" {
			t.Fatalf("transfer failure event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing transfer failure event")
	}

	runtime.mu.Lock()
	runtime.transferErr = nil
	runtime.mu.Unlock()
	retry, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
	_ = readSessionWelcome(t, retry, codec)
}
