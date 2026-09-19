package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestAmmunitionV33RoundTrip(t *testing.T) {
	codec := Codec{}
	messages := []protocol.Message{
		protocol.ClientAmmunitionCommand{Operation: protocol.AmmunitionOperationSelect, ItemArchetypeID: "item_silver_arrow"},
		protocol.AmmunitionResult{ClientActionSequence: 7, Operation: protocol.AmmunitionOperationSelect, Outcome: protocol.AmmunitionOutcomeSelected, ItemArchetypeID: "item_silver_arrow"},
		protocol.AmmunitionState{SelectedItemArchetypeID: "item_silver_arrow"},
		protocol.AmmunitionState{},
	}
	for _, want := range messages {
		data, err := codec.Marshal(want)
		if err != nil { t.Fatal(err) }
		got, err := codec.Unmarshal(want.Type(), data)
		if err != nil { t.Fatal(err) }
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip %T got=%#v want=%#v data=%s", want, got, want, data)
		}
	}
}

func TestAmmunitionV33StrictDecodeRejectsClientOwnedState(t *testing.T) {
	codec := Codec{}
	if _, err := codec.Unmarshal(protocol.MessageClientAmmunitionCommand, []byte(`{"operation":"select","item_archetype_id":"item_silver_arrow","selected":true}`)); err == nil {
		t.Fatal("expected unknown selected field rejection")
	}
}
