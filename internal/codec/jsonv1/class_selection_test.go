package jsonv1

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestInitialClassSelectionMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	tests := []protocol.Message{
		protocol.ClientInitialClassSelection{ClassID: "class_ranger"},
		protocol.CharacterClassState{ClassID: ""},
		protocol.CharacterClassState{ClassID: "class_oathguard"},
		protocol.InitialClassSelectionResult{ClientActionSequence: 17, ClassID: "class_ranger", Outcome: protocol.InitialClassSelectionCommitted},
		protocol.InitialClassSelectionResult{ClientActionSequence: 18, Outcome: protocol.InitialClassSelectionRejected, Reason: protocol.InitialClassSelectionInvalidClass},
	}
	for _, want := range tests {
		data, err := codec.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %T: %v", want, err)
		}
		got, err := codec.Unmarshal(want.Type(), data)
		if err != nil {
			t.Fatalf("unmarshal %T: %v payload=%s", want, err, data)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("roundtrip %T got=%#v want=%#v", want, got, want)
		}
	}
}

func TestInitialClassSelectionRejectsUnknownJSONField(t *testing.T) {
	_, err := (Codec{}).Unmarshal(protocol.MessageClientInitialClassSelection, []byte(`{"class_id":"class_ranger","client_decides":true}`))
	if err == nil || errors.Is(err, ErrUnsupportedMessage) {
		t.Fatalf("expected strict schema error, got %v", err)
	}
}
