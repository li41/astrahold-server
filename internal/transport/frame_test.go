package transport

import (
	"bytes"
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestFrameRoundTrip(t *testing.T) {
	in := Frame{MessageType: protocol.MessageWorldSnapshot, Delivery: protocol.DeliveryRealtimeSequenced, Flags: 3, Sequence: 42, ServerTick: 99, Payload: []byte{1, 2, 3, 4}}
	data, err := EncodeFrame(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.MessageType != in.MessageType || out.Delivery != in.Delivery || out.Flags != in.Flags || out.Sequence != in.Sequence || out.ServerTick != in.ServerTick || !bytes.Equal(out.Payload, in.Payload) {
		t.Fatalf("roundtrip mismatch: %#v", out)
	}
}
func TestDecodeRejectsBadMagic(t *testing.T) {
	data, _ := EncodeFrame(Frame{MessageType: protocol.MessageEntitySpawn, Delivery: protocol.DeliveryReliableOrdered})
	data[0] = 0
	if _, err := DecodeFrame(data); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("expected bad magic, got %v", err)
	}
}

type stubCodec struct{}

func (stubCodec) Marshal(message protocol.Message) ([]byte, error) {
	return []byte{byte(message.Type())}, nil
}

func (stubCodec) Unmarshal(messageType protocol.MessageType, _ []byte) (protocol.Message, error) {
	if messageType == protocol.MessageEntityDespawn {
		return protocol.EntityDespawn{EntityID: 9}, nil
	}
	return protocol.WorldSnapshot{}, nil
}

func TestEnvelopeCodecBoundary(t *testing.T) {
	in := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   7,
		ServerTick: 88,
		Message:    protocol.EntityDespawn{EntityID: 9},
	}
	data, err := EncodeEnvelope(in, stubCodec{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeEnvelope(data, stubCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Delivery != in.Delivery || out.Sequence != 7 || out.ServerTick != 88 || out.Message.Type() != protocol.MessageEntityDespawn {
		t.Fatalf("unexpected envelope: %#v", out)
	}
}
