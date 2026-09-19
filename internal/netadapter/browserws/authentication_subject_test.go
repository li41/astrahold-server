package browserws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestHandlerTrustedE2EBootstrapCarriesServerAuthenticationSubject(t *testing.T) {
	runtime := newFakeRuntime()
	config := DefaultConfig()
	config.WorldIdentity = testWorldIdentity()
	identity, err := characteridentity.NewTrusted("e2e-auth-subject")
	if err != nil {
		t.Fatal(err)
	}
	config.TrustedE2EBootstrapFactory = func(session.ID, world.EntityID) (TrustedE2EBootstrap, error) {
		return TrustedE2EBootstrap{
			Identity:              identity,
			AuthenticationSubject: "1",
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID:   identity.ID,
				Revision:      1,
				World:         config.WorldIdentity,
				HP:            1000,
				MaxHP:         1000,
				MP:            100,
				MaxMP:         100,
				PrimaryStats:  characterstats.DefaultPrimary(),
				Transform:     world.Transform{Position: world.Position{Layer: 0}},
				Inventory:     characterstate.InventoryState{Initialized: true},
				Warehouse:     characterstate.EmptyWarehouseState(),
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
	_ = readEnvelope(t, ctx, conn, gamev1.Codec{})

	select {
	case join := <-runtime.joins:
		if got := join.Session.AuthenticationSubject(); got != "1" {
			t.Fatalf("authentication subject=%q want=1", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for trusted E2E join")
	}

	frame, err := transport.EncodeEnvelope(protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: 1,
		Message:  protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenPersonal},
	}, gamev1.Codec{})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatal(err)
	}
}
