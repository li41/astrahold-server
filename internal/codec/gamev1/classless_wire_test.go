package gamev1

import (
	"errors"
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
	message := protocol.CharacterTargetResourceState{
		SourceEntityID: 7,
		TargetEntityID: 9,
		ResourceID:     "flaw",
		Current:        2,
		Max:            3,
	}
	codec := Codec{}
	payload, err := codec.Marshal(message)
	if err != nil { t.Fatal(err) }
	const wantPayload = `{"source_entity_id":7,"target_entity_id":9,"resource_id":"flaw","current":2,"max":3}`
	if string(payload) != wantPayload {
		t.Fatalf("payload=%s want=%s", payload, wantPayload)
	}
	if _, err := codec.Unmarshal(protocol.MessageCharacterTargetResourceState, payload); !errors.Is(err, jsonv1.ErrUnsupportedMessage) {
		t.Fatalf("inbound Type118 err=%v want ErrUnsupportedMessage", err)
	}
}
