package jsonv1

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.Envelope{
		Delivery:   protocol.DeliveryRealtimeSequenced,
		Sequence:   9,
		ServerTick: 123,
		Message: protocol.PositionCorrection{
			Tick: 123, EntityID: 5,
			Position: world.Position{X: 1.25, Y: 2.5, Z: -3.75, Layer: 4},
			Yaw: 0.5, LastProcessedInputSequence: 77,
		},
	}
	data, err := transport.EncodeEnvelope(want, codec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := transport.DecodeEnvelope(data, codec)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}

func TestClientMovePayloadDoesNotContainSequence(t *testing.T) {
	data, err := (Codec{}).Marshal(protocol.ClientMoveInput{DirectionX: 1, DirectionZ: -1})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"dx":1,"dz":-1}` {
		t.Fatalf("payload=%s", data)
	}
}

func TestUnknownFieldIsRejected(t *testing.T) {
	_, err := (Codec{}).Unmarshal(protocol.MessageClientMoveInput, []byte(`{"dx":1,"dz":0,"sequence":99}`))
	if err == nil || errors.Is(err, ErrUnsupportedMessage) {
		t.Fatalf("expected schema error, got %v", err)
	}
}
