package tcpudp

import (
	"context"
	"errors"
	"net"
	"sync"
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

type transferCall struct {
	expected    worldruntime.SessionOwnershipFence
	replacement *session.Session
	result      worldruntime.SessionOwnershipFence
}

type takeoverRuntime struct {
	*fakeRuntime
	mu           sync.Mutex
	active       worldruntime.SessionOwnershipFence
	transferErr  error
	transfers    chan transferCall
	fencedLeaves chan worldruntime.SessionOwnershipFence
}

func newTakeoverRuntime() *takeoverRuntime {
	return &takeoverRuntime{
		fakeRuntime:  newFakeRuntime(),
		transfers:    make(chan transferCall, 4),
		fencedLeaves: make(chan worldruntime.SessionOwnershipFence, 4),
	}
}

func (r *takeoverRuntime) AwaitCharacterConnectionPlan(_ context.Context, identity characteridentity.Binding) (worldruntime.CharacterConnectionPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active.Valid() {
		return worldruntime.CharacterConnectionPlan{Ownership: r.active}, nil
	}
	return worldruntime.CharacterConnectionPlan{
		AdmissionLease: worldruntime.CharacterAdmissionLease{CharacterID: identity.ID, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)},
	}, nil
}

func (r *takeoverRuntime) AwaitJoinOwned(ctx context.Context, request worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error) {
	ownership, err := r.fakeRuntime.AwaitJoinOwned(ctx, request)
	if err != nil {
		return worldruntime.SessionOwnershipFence{}, err
	}
	r.mu.Lock()
	r.active = ownership
	r.mu.Unlock()
	return ownership, nil
}

func (r *takeoverRuntime) AwaitOwnershipTransfer(_ context.Context, expected worldruntime.SessionOwnershipFence, replacement *session.Session) (worldruntime.SessionOwnershipFence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.transferErr != nil {
		return worldruntime.SessionOwnershipFence{}, r.transferErr
	}
	if r.active != expected {
		return worldruntime.SessionOwnershipFence{}, worldruntime.ErrCharacterOwnershipFenceStale
	}
	result := worldruntime.SessionOwnershipFence{
		SessionID:    replacement.ID,
		EntityID:     replacement.EntityID,
		CharacterID: replacement.CharacterIdentity.ID,
		Epoch:        expected.Epoch + 1,
	}
	r.active = result
	r.transfers <- transferCall{expected: expected, replacement: replacement, result: result}
	return result, nil
}

func (r *takeoverRuntime) EnqueueFencedLeave(fence worldruntime.SessionOwnershipFence) error {
	r.fencedLeaves <- fence
	return nil
}

func allowCharacterTakeoverForTest(context.Context, CharacterTakeoverRequest) error { return nil }

func TestTrustedActiveTakeoverReusesEntityEvictsOldPeerAndRoutesNewOwner(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:tcp-takeover")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	cfg.CharacterTakeoverAuthorizer = allowCharacterTakeoverForTest
	var restoreCalls atomic.Int32
	cfg.CharacterRestoreFactory = func(characteridentity.Binding) (worldruntime.CharacterRestore, bool, error) {
		restoreCalls.Add(1)
		return worldruntime.CharacterRestore{}, false, nil
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

	newConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer newConn.Close()
	newWelcome := readSessionWelcome(t, newConn, codec)

	if newWelcome.SessionID == oldWelcome.SessionID {
		t.Fatalf("takeover reused SessionID=%d", newWelcome.SessionID)
	}
	if newWelcome.EntityID != oldWelcome.EntityID {
		t.Fatalf("takeover entity=%d want existing=%d", newWelcome.EntityID, oldWelcome.EntityID)
	}
	if newWelcome.RealtimeToken == oldWelcome.RealtimeToken {
		t.Fatal("takeover reused realtime token")
	}
	if restoreCalls.Load() != 1 {
		t.Fatalf("restore calls=%d want=1 inactive join only", restoreCalls.Load())
	}

	var transfer transferCall
	select {
	case transfer = <-runtime.transfers:
	case <-time.After(time.Second):
		t.Fatal("ownership transfer not observed")
	}
	if uint64(transfer.expected.SessionID) != oldWelcome.SessionID || transfer.expected.EntityID != oldWelcome.EntityID {
		t.Fatalf("transfer expected=%#v old welcome=%#v", transfer.expected, oldWelcome)
	}
	if uint64(transfer.replacement.ID) != newWelcome.SessionID || transfer.replacement.EntityID != oldWelcome.EntityID {
		t.Fatalf("replacement session=%d entity=%d welcome=%#v", transfer.replacement.ID, transfer.replacement.EntityID, newWelcome)
	}

	_ = oldConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(oldConn, codec); err == nil {
		t.Fatal("old TCP peer remained open after takeover")
	}
	select {
	case fence := <-runtime.fencedLeaves:
		t.Fatalf("takeover eviction enqueued old Leave=%#v", fence)
	default:
	}

	actionMessage := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(newConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 9, Message: actionMessage}, codec); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != newWelcome.SessionID || call.seq != 9 || call.action != actionMessage {
			t.Fatalf("new action route=%#v", call)
		}
	case <-time.After(time.Second):
		t.Fatal("new owner action not routed")
	}

	oldToken, err := ParseToken(oldWelcome.RealtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	newToken, err := ParseToken(newWelcome.RealtimeToken)
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.DialUDP("udp", nil, server.UDPAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	oldPacket, err := EncodeDatagram(oldToken, protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 3, Message: protocol.ClientMoveInput{DirectionX: 1}}, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := udpConn.Write(oldPacket); err != nil {
		t.Fatal(err)
	}
	select {
	case move := <-runtime.moves:
		t.Fatalf("retired old UDP token routed move=%#v", move)
	case <-time.After(50 * time.Millisecond):
	}

	newPacket, err := EncodeDatagram(newToken, protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 4, Message: protocol.ClientMoveInput{DirectionZ: 1}}, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := udpConn.Write(newPacket); err != nil {
		t.Fatal(err)
	}
	select {
	case move := <-runtime.moves:
		if uint64(move.id) != newWelcome.SessionID || move.seq != 4 || move.input.DirectionZ != 1 {
			t.Fatalf("new UDP route=%#v", move)
		}
	case <-time.After(time.Second):
		t.Fatal("new owner UDP move not routed")
	}

	_ = newConn.Close()
	select {
	case fence := <-runtime.fencedLeaves:
		if fence != transfer.result {
			t.Fatalf("new owner leave=%#v want=%#v", fence, transfer.result)
		}
	case <-time.After(time.Second):
		t.Fatal("new owner close did not enqueue fenced Leave")
	}
}

func TestTrustedActiveTakeoverTransferFailureKeepsOldPeerActive(t *testing.T) {
	runtime := newTakeoverRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, _ := characteridentity.NewTrusted("character:tcp-takeover-fail")
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
	oldWelcome := readSessionWelcome(t, oldConn, codec)

	runtime.mu.Lock()
	runtime.transferErr = errors.New("test transfer rejection")
	runtime.mu.Unlock()
	newConn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer newConn.Close()
	_ = newConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := transport.ReadEnvelope(newConn, codec); err == nil {
		t.Fatal("failed takeover received SessionWelcome")
	}

	actionMessage := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := transport.WriteEnvelope(oldConn, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 5, Message: actionMessage}, codec); err != nil {
		t.Fatalf("old peer was evicted on failed transfer: %v", err)
	}
	select {
	case call := <-runtime.actions:
		if uint64(call.id) != oldWelcome.SessionID || call.seq != 5 {
			t.Fatalf("old action route=%#v welcome=%#v", call, oldWelcome)
		}
	case <-time.After(time.Second):
		t.Fatal("old peer did not remain active after failed transfer")
	}
	select {
	case fence := <-runtime.fencedLeaves:
		t.Fatalf("failed transfer enqueued Leave=%#v", fence)
	default:
	}
}

func TestRetireTakenOverPeerBeforeOwnershipPublicationSuppressesLateLeave(t *testing.T) {
	runtime := newTakeoverRuntime()
	server := NewServer(DefaultConfig(), runtime, gamev1.Codec{})
	identity, err := characteridentity.NewTrusted("character:prepublish-retire")
	if err != nil {
		t.Fatal(err)
	}
	expected := worldruntime.SessionOwnershipFence{SessionID: 9, EntityID: 11, CharacterID: identity.ID, Epoch: 3}
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer right.Close()
	connection := newClientConnection(left, nil, token, gamev1.Codec{}, 8, &server.metrics)
	old := &peer{sessionID: expected.SessionID, entityID: expected.EntityID, token: token, conn: connection, ingress: server.ingress}
	server.mu.Lock()
	server.peers[token] = old
	server.mu.Unlock()

	// Model the F.20 window where worldruntime has already published active ownership but
	// the old transport goroutine has not yet copied the fence or stored joined=true.
	server.retireTakenOverPeer(expected)
	server.mu.RLock()
	_, stillPresent := server.peers[token]
	server.mu.RUnlock()
	if stillPresent {
		t.Fatal("pre-publication old peer remained in transport map")
	}

	// The old handle can still return from its already-committed join after retirement.
	// Publishing ownership/joined and calling closePeer again must not enqueue any Leave.
	old.ownership = expected
	old.joined.Store(true)
	server.closePeer(old, "late_join_completion", net.ErrClosed)
	select {
	case fence := <-runtime.fencedLeaves:
		t.Fatalf("late old join enqueued fenced Leave=%#v", fence)
	default:
	}
	select {
	case id := <-runtime.leaves:
		t.Fatalf("late old join enqueued legacy Leave=%d", id)
	default:
	}
}

func readSessionWelcome(t *testing.T, conn net.Conn, codec transport.PayloadCodec) protocol.SessionWelcome {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	envelope, err := transport.ReadEnvelope(conn, codec)
	if err != nil {
		t.Fatal(err)
	}
	welcome, ok := envelope.Message.(protocol.SessionWelcome)
	if !ok {
		t.Fatalf("unexpected welcome=%#v", envelope.Message)
	}
	return welcome
}
