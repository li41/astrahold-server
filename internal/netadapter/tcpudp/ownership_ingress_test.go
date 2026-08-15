package tcpudp

import (
	"context"
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

type ownershipIngressCall struct {
	kind  string
	fence worldruntime.SessionOwnershipFence
}

type ownershipRecordingRuntime struct {
	*fakeRuntime
	ownership worldruntime.SessionOwnershipFence
	calls     chan ownershipIngressCall
}

func (r *ownershipRecordingRuntime) AwaitJoinOwned(_ context.Context, request worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error) {
	if err := r.fakeRuntime.EnqueueJoin(request); err != nil {
		return worldruntime.SessionOwnershipFence{}, err
	}
	r.ownership = worldruntime.SessionOwnershipFence{
		SessionID:   request.Session.ID,
		EntityID:    request.Session.EntityID,
		CharacterID: request.Session.CharacterIdentity.ID,
		Epoch:       91,
	}
	return r.ownership, nil
}

func (r *ownershipRecordingRuntime) EnqueueFencedMove(fence worldruntime.SessionOwnershipFence, _ uint32, _ protocol.ClientMoveInput) error {
	r.calls <- ownershipIngressCall{kind: "move", fence: fence}
	return nil
}

func (r *ownershipRecordingRuntime) EnqueueFencedUseAction(fence worldruntime.SessionOwnershipFence, _ uint32, _ protocol.ClientUseAction) error {
	r.calls <- ownershipIngressCall{kind: "action", fence: fence}
	return nil
}

func (r *ownershipRecordingRuntime) EnqueueFencedLeave(fence worldruntime.SessionOwnershipFence) error {
	r.calls <- ownershipIngressCall{kind: "leave", fence: fence}
	return nil
}

func TestTrustedPeerUsesSameOwnershipFenceForTCPUDPAndLeave(t *testing.T) {
	runtime := &ownershipRecordingRuntime{fakeRuntime: newFakeRuntime(), calls: make(chan ownershipIngressCall, 3)}
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:ownership-ingress")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	tcpConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := tcpConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	welcomeEnvelope, err := transport.ReadEnvelope(tcpConn, codec)
	if err != nil {
		t.Fatal(err)
	}
	welcome, ok := welcomeEnvelope.Message.(protocol.SessionWelcome)
	if !ok {
		t.Fatalf("welcome=%#v", welcomeEnvelope.Message)
	}

	action := protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 1,
		Message: protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"},
	}
	if err := transport.WriteEnvelope(tcpConn, action, codec); err != nil {
		t.Fatal(err)
	}
	actionCall := waitOwnershipIngressCall(t, runtime.calls, "action")

	token, err := ParseToken(welcome.RealtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.DialUDP("udp", nil, server.UDPAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	packet, err := EncodeDatagram(token, protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 1,
		Message:  protocol.ClientMoveInput{DirectionX: 1},
	}, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := udpConn.Write(packet); err != nil {
		t.Fatal(err)
	}
	moveCall := waitOwnershipIngressCall(t, runtime.calls, "move")

	if err := tcpConn.Close(); err != nil {
		t.Fatal(err)
	}
	leaveCall := waitOwnershipIngressCall(t, runtime.calls, "leave")

	for _, call := range []ownershipIngressCall{actionCall, moveCall, leaveCall} {
		if call.fence != runtime.ownership {
			t.Fatalf("%s fence=%#v want=%#v", call.kind, call.fence, runtime.ownership)
		}
	}
	if runtime.ownership.SessionID != session.ID(welcome.SessionID) || runtime.ownership.EntityID != welcome.EntityID || runtime.ownership.CharacterID != identity.ID {
		t.Fatalf("ownership=%#v welcome=%#v", runtime.ownership, welcome)
	}
}

func waitOwnershipIngressCall(t *testing.T, calls <-chan ownershipIngressCall, want string) ownershipIngressCall {
	t.Helper()
	select {
	case call := <-calls:
		if call.kind != want {
			t.Fatalf("ownership call=%s want=%s", call.kind, want)
		}
		return call
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s ownership call", want)
		return ownershipIngressCall{}
	}
}
