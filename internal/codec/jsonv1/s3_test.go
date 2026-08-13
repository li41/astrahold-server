package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestS3WorldMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	messages := []protocol.Message{
		protocol.SessionWelcome{
			SessionID: 1, EntityID: 2, RealtimePort: 7778, RealtimeToken: "00112233445566778899aabbccddeeff",
			TickRateHz: 20, SnapshotRateHz: 10,
			World: protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3a-001", GameplaySHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		},
		protocol.WorldDynamicState{Revision: 7, Blockers: []protocol.WorldBlockerState{{ID: "main-gate", Enabled: false}}},
	}

	for _, want := range messages {
		payload, err := codec.Marshal(want)
		if err != nil { t.Fatalf("Marshal(%T): %v", want, err) }
		got, err := codec.Unmarshal(want.Type(), payload)
		if err != nil { t.Fatalf("Unmarshal(%T): %v", want, err) }
		if !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v want=%#v", got, want) }
	}
}
