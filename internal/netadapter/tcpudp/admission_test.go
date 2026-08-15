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

type controlledRuntime struct {
	*fakeRuntime
	admissionErr error
	joinErr      error
}

func (r *controlledRuntime) AwaitCharacterAdmission(context.Context, characteridentity.Binding) error {
	return r.admissionErr
}

func (r *controlledRuntime) AwaitJoin(_ context.Context, request worldruntime.JoinRequest) error {
	if r.joinErr != nil {
		return r.joinErr
	}
	return r.fakeRuntime.EnqueueJoin(request)
}

func TestTrustedAdmissionFailurePreventsRestoreAndWelcome(t *testing.T) {
	runtime := &controlledRuntime{fakeRuntime: newFakeRuntime(), admissionErr: worldruntime.ErrCharacterIdentityActive}
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:admission-reject")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	restoreCalled := false
	cfg.CharacterRestoreFactory = func(characteridentity.Binding) (worldruntime.CharacterRestore, bool, error) {
		restoreCalled = true
		return worldruntime.CharacterRestore{}, false, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)
	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ReadEnvelope(conn, codec); err == nil {
		t.Fatal("rejected admission received SessionWelcome")
	}
	if restoreCalled {
		t.Fatal("restore lookup ran before rejected world-owner admission")
	}
	select {
	case join := <-runtime.joins:
		t.Fatalf("unexpected join=%#v", join)
	default:
	}
}

func TestWorldJoinFailurePreventsSessionWelcome(t *testing.T) {
	joinFailure := errors.New("test join rejection")
	runtime := &controlledRuntime{fakeRuntime: newFakeRuntime(), joinErr: joinFailure}
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ReadEnvelope(conn, codec); err == nil {
		t.Fatal("failed world join received SessionWelcome")
	}
	select {
	case id := <-runtime.leaves:
		t.Fatalf("join rejection enqueued leave for uncommitted session=%d", id)
	default:
	}
}

func TestClosePeerCanEnqueueLeaveAfterJoinCommitsDuringCloseRace(t *testing.T) {
	runtime := newFakeRuntime()
	server := NewServer(DefaultConfig(), runtime, gamev1.Codec{})
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer right.Close()
	connection := newClientConnection(left, nil, token, gamev1.Codec{}, 8, &server.metrics)
	p := &peer{sessionID: 9, entityID: 11, token: token, conn: connection}
	server.mu.Lock()
	server.peers[token] = p
	server.mu.Unlock()

	server.closePeer(p, "server_close", nil)
	p.joined.Store(true)
	server.closePeer(p, "welcome_write", net.ErrClosed)

	select {
	case id := <-runtime.leaves:
		if id != p.sessionID {
			t.Fatalf("leave id=%d want=%d", id, p.sessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("late committed join did not enqueue leave")
	}
}

func startTCPUDPTestServer(t *testing.T, cfg Config, runtime RuntimeSink, codec transport.PayloadCodec) (*Server, context.CancelFunc, <-chan error) {
	t.Helper()
	server := NewServer(cfg, runtime, codec)
	if err := server.Open(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()
	return server, cancel, serveDone
}

func stopTCPUDPTestServer(t *testing.T, server *Server, cancel context.CancelFunc, serveDone <-chan error) {
	t.Helper()
	cancel()
	_ = server.Close()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not stop")
	}
}
