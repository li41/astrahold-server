package gamev1

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestProtocolV34RejectsRetiredClassWireIDs(t *testing.T) {
	if protocol.Version != 34 {
		t.Fatalf("protocol version=%d want=34", protocol.Version)
	}
	codec := Codec{}
	for _, messageType := range []protocol.MessageType{9, 115, 116, 117} {
		if _, err := codec.Unmarshal(messageType, []byte("{}")); !errors.Is(err, jsonv1.ErrUnsupportedMessage) {
			t.Fatalf("retired message type %d err=%v want ErrUnsupportedMessage", messageType, err)
		}
	}
}

func TestProtocolV34KeepsTargetResourceType118(t *testing.T) {
	if protocol.MessageCharacterTargetResourceState != 118 {
		t.Fatalf("target resource type=%d want=118", protocol.MessageCharacterTargetResourceState)
	}
	want := protocol.CharacterTargetResourceState{
		SourceEntityID: 7,
		TargetEntityID: 9,
		ResourceID:     "flaw",
		Current:        2,
		Max:            3,
	}
	codec := Codec{}
	payload, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageCharacterTargetResourceState, payload)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded=%#v want=%#v", decoded, want)
	}
}
