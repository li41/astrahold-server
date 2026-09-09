package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestClientInteractNPCRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ClientInteractNPC{NPCEntityID: world.EntityID(7001)}
	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("marshal NPC intent: %v", err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageClientInteractNPC, payload)
	if err != nil {
		t.Fatalf("unmarshal NPC intent: %v", err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("round trip = %#v, want %#v", decoded, want)
	}
}

func TestNPCInteractionRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.NPCInteraction{
		NPCEntityID:    world.EntityID(7001),
		NPCArchetypeID: "npc_emberwatch_warden",
		DisplayName:    "Warden Sera",
		Text:           "The eastern road is secure.",
	}
	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("marshal NPC interaction: %v", err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageNPCInteraction, payload)
	if err != nil {
		t.Fatalf("unmarshal NPC interaction: %v", err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("round trip = %#v, want %#v", decoded, want)
	}
}

func TestClientInteractNPCStrictDecodeRejectsUnknownFields(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageClientInteractNPC, []byte(`{"npc_entity_id":7001,"client_range":999}`))
	if err == nil {
		t.Fatal("expected strict decoder to reject unknown NPC intent field")
	}
}
