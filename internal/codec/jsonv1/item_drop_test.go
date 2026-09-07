package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestClientPickupItemRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ClientPickupItem{DropEntityID: world.EntityID(8_000_000_000_000_123)}

	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("marshal pickup item: %v", err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageClientPickupItem, payload)
	if err != nil {
		t.Fatalf("unmarshal pickup item: %v", err)
	}
	got, ok := decoded.(protocol.ClientPickupItem)
	if !ok {
		t.Fatalf("decoded type = %T, want protocol.ClientPickupItem", decoded)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestClientPickupItemStrictDecodeRejectsUnknownFields(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageClientPickupItem, []byte(`{"drop_entity_id":8000000000000123,"quantity":99}`))
	if err == nil {
		t.Fatal("expected strict decoder to reject client-supplied pickup quantity")
	}
}

func TestClientUseItemRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}

	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("marshal use item: %v", err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageClientUseItem, payload)
	if err != nil {
		t.Fatalf("unmarshal use item: %v", err)
	}
	got, ok := decoded.(protocol.ClientUseItem)
	if !ok {
		t.Fatalf("decoded type = %T, want protocol.ClientUseItem", decoded)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestClientUseItemStrictDecodeRejectsClientRestoreAmount(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageClientUseItem, []byte(`{"item_archetype_id":"item_minor_healing_potion","restore_amount":999999}`))
	if err == nil {
		t.Fatal("expected strict decoder to reject client-supplied restore amount")
	}
}
