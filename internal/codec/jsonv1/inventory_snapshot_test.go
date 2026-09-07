package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestInventorySnapshotRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.InventorySnapshot{
		Revision:           7,
		CurrentCarryWeight: 19,
		MaxCarryWeight:     100,
		Items: []protocol.InventoryItemStack{
			{ArchetypeID: "item_minor_healing_potion", Quantity: 3},
			{ArchetypeID: "item_minor_mana_potion", Quantity: 2},
			{ArchetypeID: "item_training_blade", Quantity: 1},
		},
	}

	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("marshal inventory snapshot: %v", err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageInventorySnapshot, payload)
	if err != nil {
		t.Fatalf("unmarshal inventory snapshot: %v", err)
	}
	got, ok := decoded.(protocol.InventorySnapshot)
	if !ok {
		t.Fatalf("decoded type = %T, want protocol.InventorySnapshot", decoded)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestInventorySnapshotStrictDecodeRejectsUnknownFields(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageInventorySnapshot, []byte(`{"revision":1,"current_carry_weight":0,"max_carry_weight":100,"items":[],"client_owned":true}`))
	if err == nil {
		t.Fatal("expected strict decoder to reject unknown inventory field")
	}
}
