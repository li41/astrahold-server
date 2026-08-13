package transport

import (
	"bytes"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestStreamEnvelopeRoundTrip(t *testing.T) {
	codec := jsonv1.Codec{}
	want := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   7,
		ServerTick: 88,
		Message: protocol.SessionWelcome{
			SessionID:      1,
			EntityID:       2,
			RealtimePort:   7778,
			RealtimeToken:  "00112233445566778899aabbccddeeff",
			TickRateHz:     20,
			SnapshotRateHz: 10,
		},
	}
	var buffer bytes.Buffer
	if err := WriteEnvelope(&buffer, want, codec); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEnvelope(&buffer, codec)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sequence != want.Sequence || got.ServerTick != want.ServerTick || got.Delivery != want.Delivery {
		t.Fatalf("envelope mismatch: %#v", got)
	}
	welcome, ok := got.Message.(protocol.SessionWelcome)
	if !ok || welcome.RealtimeToken != "00112233445566778899aabbccddeeff" {
		t.Fatalf("welcome mismatch: %#v", got.Message)
	}
}
