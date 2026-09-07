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

type fullFakeRuntime struct {
	*fakeRuntime
	equipment chan protocol.ClientEquipmentCommand
	pickups   chan protocol.ClientPickupItem
	npcs      chan protocol.ClientInteractNPC
	shops     chan protocol.ClientShopCommand
	respawns  chan protocol.ClientRespawnRequest
}

func newFullFakeRuntime() *fullFakeRuntime {
	return &fullFakeRuntime{
		fakeRuntime: newFakeRuntime(),
		equipment:   make(chan protocol.ClientEquipmentCommand, 4),
		pickups:     make(chan protocol.ClientPickupItem, 4),
		npcs:        make(chan protocol.ClientInteractNPC, 4),
		shops:       make(chan protocol.ClientShopCommand, 4),
		respawns:    make(chan protocol.ClientRespawnRequest, 4),
	}
}

func (r *fullFakeRuntime) EnqueueEquipmentCommand(_ session.ID, _ uint32, command protocol.ClientEquipmentCommand) error {
	r.equipment <- command
	return nil
}

func (r *fullFakeRuntime) EnqueuePickupItem(_ session.ID, _ uint32, intent protocol.ClientPickupItem) error {
	r.pickups <- intent
	return nil
}

func (r *fullFakeRuntime) EnqueueInteractNPC(_ session.ID, _ uint32, intent protocol.ClientInteractNPC) error {
	r.npcs <- intent
	return nil
}

func (r *fullFakeRuntime) EnqueueShopCommand(_ session.ID, _ uint32, intent protocol.ClientShopCommand) error {
	r.shops <- intent
	return nil
}

func (r *fullFakeRuntime) EnqueueRespawnRequest(_ session.ID, _ uint32, intent protocol.ClientRespawnRequest) error {
	r.respawns <- intent
	return nil
}

func TestHandlerRoutesModernReliableIntentsThroughGateway(t *testing.T) {
	t.Parallel()

	runtime := newFullFakeRuntime()
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
	_ = readEnvelope(t, ctx, conn, codec) // SessionWelcome
	_ = readEnvelope(t, ctx, conn, codec) // initial EntitySpawn from fake runtime

	writeBrowserIntent(t, ctx, conn, codec, 1, protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotMainHand,
		ItemArchetypeID: "item_training_blade",
	})
	select {
	case got := <-runtime.equipment:
		if got.ItemArchetypeID != "item_training_blade" {
			t.Fatalf("equipment intent changed: %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for equipment intent")
	}

	writeBrowserIntent(t, ctx, conn, codec, 2, protocol.ClientPickupItem{DropEntityID: 5001})
	select {
	case got := <-runtime.pickups:
		if got.DropEntityID != 5001 {
			t.Fatalf("pickup intent changed: %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for pickup intent")
	}

	writeBrowserIntent(t, ctx, conn, codec, 3, protocol.ClientInteractNPC{NPCEntityID: 7001})
	select {
	case got := <-runtime.npcs:
		if got.NPCEntityID != 7001 {
			t.Fatalf("npc intent changed: %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for npc intent")
	}

	writeBrowserIntent(t, ctx, conn, codec, 4, protocol.ClientShopCommand{
		Operation:   protocol.ShopOperationOpen,
		NPCEntityID: 7001,
	})
	select {
	case got := <-runtime.shops:
		if got.Operation != protocol.ShopOperationOpen || got.NPCEntityID != 7001 {
			t.Fatalf("shop intent changed: %#v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for shop intent")
	}

	writeBrowserIntent(t, ctx, conn, codec, 5, protocol.ClientRespawnRequest{})
	select {
	case <-runtime.respawns:
	case <-ctx.Done():
		t.Fatal("timed out waiting for respawn intent")
	}
}

func writeBrowserIntent(t *testing.T, ctx context.Context, conn *websocket.Conn, codec gamev1.Codec, sequence uint32, message protocol.Message) {
	t.Helper()
	frame, err := transport.EncodeEnvelope(protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: sequence,
		Message:  message,
	}, codec)
	if err != nil {
		t.Fatalf("encode client intent %T: %v", message, err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write client intent %T: %v", message, err)
	}
}
