package browserws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type moveRecord struct {
	sessionID session.ID
	sequence  uint32
	input     protocol.ClientMoveInput
}

type actionRecord struct {
	sessionID session.ID
	sequence  uint32
	action    protocol.ClientUseAction
}

type fakeRuntime struct {
	moves   chan moveRecord
	actions chan actionRecord
	leaves  chan session.ID
	joins   chan worldruntime.JoinRequest
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		moves:   make(chan moveRecord, 4),
		actions: make(chan actionRecord, 4),
		leaves:  make(chan session.ID, 4),
		joins:   make(chan worldruntime.JoinRequest, 4),
	}
}

func (r *fakeRuntime) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	r.moves <- moveRecord{sessionID: id, sequence: sequence, input: input}
	return nil
}

func (r *fakeRuntime) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	r.actions <- actionRecord{sessionID: id, sequence: sequence, action: action}
	return nil
}

func (r *fakeRuntime) AwaitJoinOwned(_ context.Context, request worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error) {
	r.joins <- request
	_ = request.Session.Connection().TrySend(protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   1,
		ServerTick: 1,
		Message: protocol.EntitySpawn{
			EntityID: request.Entity.ID,
			Kind:     request.Entity.Kind,
			Transform: protocol.EntityTransform{
				EntityID: request.Entity.ID,
				Tick:     1,
				Position: request.Entity.Transform.Position,
				Yaw:      request.Entity.Transform.Yaw,
			},
			ArchetypeID: "test-player",
		},
	})
	return worldruntime.SessionOwnershipFence{}, nil
}

func (r *fakeRuntime) EnqueueLeave(id session.ID) error {
	r.leaves <- id
	return nil
}

func testWorldIdentity() protocol.WorldIdentity {
	return protocol.WorldIdentity{
		WorldID:        "emberwatch-test",
		Revision:       "browserws-test-001",
		GameplaySHA256: strings.Repeat("a", 64),
	}
}

func TestHandlerWelcomeSpawnAndRealtimeIngress(t *testing.T) {
	t.Parallel()

	runtime := newFakeRuntime()
	config := DefaultConfig()
	config.WorldIdentity = testWorldIdentity()
	codec := gamev1.Codec{}
	httpServer := httptest.NewServer(NewHandler(config, runtime, codec))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()

	welcome := readEnvelope(t, ctx, conn, codec)
	message, ok := welcome.Message.(protocol.SessionWelcome)
	if !ok {
		t.Fatalf("welcome message type = %T", welcome.Message)
	}
	if welcome.Delivery != protocol.DeliveryReliableOrdered || message.SessionID == 0 || message.EntityID == 0 {
		t.Fatalf("invalid welcome: %#v", welcome)
	}
	if message.RealtimePort != 0 || message.RealtimeToken != "" {
		t.Fatalf("websocket welcome must not advertise UDP: %#v", message)
	}
	if message.World != config.WorldIdentity {
		t.Fatalf("world identity mismatch: %#v", message.World)
	}

	spawn := readEnvelope(t, ctx, conn, codec)
	spawnMessage, ok := spawn.Message.(protocol.EntitySpawn)
	if !ok || spawnMessage.EntityID != message.EntityID {
		t.Fatalf("spawn mismatch: %#v", spawn)
	}

	clientFrame, err := transport.EncodeEnvelope(protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 1,
		Message:  protocol.ClientMoveInput{DirectionX: 0.5, DirectionZ: -1},
	}, codec)
	if err != nil {
		t.Fatalf("encode client move: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, clientFrame); err != nil {
		t.Fatalf("write client move: %v", err)
	}

	select {
	case move := <-runtime.moves:
		if move.sequence != 1 || move.sessionID != session.ID(message.SessionID) || move.input.DirectionX != 0.5 || move.input.DirectionZ != -1 {
			t.Fatalf("unexpected move ingress: %#v", move)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for authoritative move ingress")
	}
}

func TestHandlerTrustedE2EBootstrapUsesServerOwnedIdentityAndClasslessRestore(t *testing.T) {
	t.Parallel()

	runtime := newFakeRuntime()
	config := DefaultConfig()
	config.WorldIdentity = testWorldIdentity()
	identity, err := characteridentity.NewTrusted("e2e-classless")
	if err != nil {
		t.Fatal(err)
	}
	config.TrustedE2EBootstrapFactory = func(session.ID, world.EntityID) (TrustedE2EBootstrap, error) {
		return TrustedE2EBootstrap{
			Identity: identity,
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID:   identity.ID,
				Revision:      1,
				World:         config.WorldIdentity,
				HP:            1000,
				MaxHP:         1000,
				MP:            100,
				MaxMP:         100,
				Transform:     world.Transform{Position: world.Position{Layer: 0}},
				Inventory:     characterstate.InventoryState{Initialized: true},
			},
		}, nil
	}

	httpServer := httptest.NewServer(NewHandler(config, runtime, gamev1.Codec{}))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()

	_ = readEnvelope(t, ctx, conn, gamev1.Codec{})
	select {
	case join := <-runtime.joins:
		if join.Session.CharacterIdentity != identity {
			t.Fatalf("identity = %#v", join.Session.CharacterIdentity)
		}
		if join.Restore == nil || join.Restore.CharacterID != identity.ID || join.Restore.LegacyRuntimeClassID != "" {
			t.Fatalf("restore = %#v", join.Restore)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for trusted E2E join")
	}
}

func TestHandlerTrustedE2EBootstrapRejectsNonLoopbackPeerBeforeFactory(t *testing.T) {
	t.Parallel()

	runtime := newFakeRuntime()
	config := DefaultConfig()
	config.WorldIdentity = testWorldIdentity()
	called := false
	config.TrustedE2EBootstrapFactory = func(session.ID, world.EntityID) (TrustedE2EBootstrap, error) {
		called = true
		return TrustedE2EBootstrap{}, nil
	}
	handler := NewHandler(config, runtime, gamev1.Codec{})
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.RemoteAddr = "203.0.113.10:4242"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d want %d", response.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("trusted E2E bootstrap factory called for non-loopback peer")
	}
}

func TestHandlerRejectsTextMessages(t *testing.T) {
	t.Parallel()

	runtime := newFakeRuntime()
	config := DefaultConfig()
	config.WorldIdentity = testWorldIdentity()
	httpServer := httptest.NewServer(NewHandler(config, runtime, gamev1.Codec{}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.CloseNow()
	_ = readEnvelope(t, ctx, conn, gamev1.Codec{})
	_ = readEnvelope(t, ctx, conn, gamev1.Codec{})

	if err := conn.Write(ctx, websocket.MessageText, []byte("not-an-astr-frame")); err != nil {
		t.Fatalf("write text message: %v", err)
	}
	_, _, err = conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusUnsupportedData {
		t.Fatalf("close status = %v, err = %v", websocket.CloseStatus(err), err)
	}
}

func readEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn, codec gamev1.Codec) protocol.Envelope {
	t.Helper()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("message type = %v", messageType)
	}
	envelope, err := transport.DecodeEnvelope(data, codec)
	if err != nil {
		t.Fatalf("decode ASTR frame: %v", err)
	}
	return envelope
}

var _ RuntimeSink = (*fakeRuntime)(nil)
var _ = world.EntityPlayer
