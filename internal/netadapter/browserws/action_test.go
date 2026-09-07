package browserws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
)

func TestHandlerReliableActionIngress(t *testing.T) {
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
	welcomeMessage, ok := welcome.Message.(protocol.SessionWelcome)
	if !ok {
		t.Fatalf("welcome message type = %T", welcome.Message)
	}
	_ = readEnvelope(t, ctx, conn, codec)

	action := protocol.ClientUseAction{
		ActionID:   "shatter-strike",
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9001",
	}
	frame, err := transport.EncodeEnvelope(protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 7,
		Message:  action,
	}, codec)
	if err != nil {
		t.Fatalf("encode client action: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write client action: %v", err)
	}

	select {
	case got := <-runtime.actions:
		if got.sessionID != session.ID(welcomeMessage.SessionID) || got.sequence != 7 || got.action != action {
			t.Fatalf("unexpected action ingress: %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for authoritative action ingress")
	}
}
