package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestWarehouseIntentRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationDeposit, ItemArchetypeID: "item_minor_healing_potion", Quantity: 3}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	message, err := codec.Unmarshal(protocol.MessageClientWarehouseCommand, data)
	if err != nil { t.Fatal(err) }
	got, ok := message.(protocol.ClientWarehouseCommand)
	if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v", message) }
}

func TestWarehouseResultRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.WarehouseResult{ClientActionSequence: 17, Operation: protocol.WarehouseOperationWithdraw, Outcome: protocol.WarehouseOutcomeRejected, Reason: protocol.WarehouseRejectionInsufficientWarehouse, ItemArchetypeID: "item_minor_healing_potion", Quantity: 4}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	message, err := codec.Unmarshal(protocol.MessageWarehouseResult, data)
	if err != nil { t.Fatal(err) }
	got, ok := message.(protocol.WarehouseResult)
	if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v", message) }
}

func TestWarehouseSnapshotRoundTripIncludesEmptyReplacement(t *testing.T) {
	codec := Codec{}
	for _, want := range []protocol.WarehouseSnapshot{
		{Items: []protocol.WarehouseItemStack{}},
		{Items: []protocol.WarehouseItemStack{{ItemArchetypeID: "item_minor_healing_potion", Quantity: 7}, {ItemArchetypeID: "item_minor_mana_potion", Quantity: 2}}},
	} {
		data, err := codec.Marshal(want)
		if err != nil { t.Fatal(err) }
		message, err := codec.Unmarshal(protocol.MessageWarehouseSnapshot, data)
		if err != nil { t.Fatal(err) }
		got, ok := message.(protocol.WarehouseSnapshot)
		if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v want=%#v data=%s", message, want, data) }
	}
}

func TestWarehouseIntentStrictDecodeRejectsUnknownField(t *testing.T) {
	codec := Codec{}
	if _, err := codec.Unmarshal(protocol.MessageClientWarehouseCommand, []byte(`{"operation":"open","account_id":"1"}`)); err == nil {
		t.Fatal("expected unknown account_id field to be rejected")
	}
}
