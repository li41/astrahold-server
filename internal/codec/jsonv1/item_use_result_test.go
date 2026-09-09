package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestItemUseResultRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ItemUseResult{
		ClientActionSequence: 17,
		ItemArchetypeID:      "item_minor_healing_potion",
		Outcome:              protocol.ItemUseOutcomeUsed,
		AppliedAmount:        100,
		CooldownReadyTick:    142,
	}
	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageItemUseResult, payload)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(protocol.ItemUseResult)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded=%#v type=%T want=%#v", decoded, decoded, want)
	}
}

func TestItemUseResultCooldownRejectionRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ItemUseResult{
		ClientActionSequence: 18,
		ItemArchetypeID:      "item_minor_mana_potion",
		Outcome:              protocol.ItemUseOutcomeRejected,
		Reason:               protocol.ItemUseRejectionCooldown,
		CooldownReadyTick:    142,
	}
	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageItemUseResult, payload)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(protocol.ItemUseResult)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded=%#v type=%T want=%#v", decoded, decoded, want)
	}
}

func TestClientUseItemStrictDecodeRejectsClientCooldownPolicy(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageClientUseItem, []byte(`{"item_archetype_id":"item_minor_healing_potion","cooldown_ticks":0,"cooldown_group":"client_override"}`))
	if err == nil {
		t.Fatal("expected strict decoder to reject client-supplied cooldown policy")
	}
}
