package tcpudp

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestDatagramRoundTrip(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	codec := jsonv1.Codec{}
	want := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 9,
		Message: protocol.ClientMoveInput{
			DirectionX: 1,
			DirectionZ: -0.5,
		},
	}
	packet, err := EncodeDatagram(token, want, codec)
	if err != nil {
		t.Fatal(err)
	}
	gotToken, got, err := DecodeDatagram(packet, codec)
	if err != nil {
		t.Fatal(err)
	}
	if gotToken != token || got.Sequence != 9 || got.Delivery != protocol.DeliveryRealtimeSequenced {
		t.Fatalf("header mismatch")
	}
	move, ok := got.Message.(protocol.ClientMoveInput)
	if !ok || move.DirectionX != 1 || move.DirectionZ != -0.5 {
		t.Fatalf("move mismatch: %#v", got.Message)
	}
	parsed, err := ParseToken(token.String())
	if err != nil || parsed != token {
		t.Fatalf("token parse failed")
	}
}

func TestDatagramRejectsOversizePayload(t *testing.T) {
	token, _ := NewToken()
	codec := largeCodec{size: MaxDatagramSize}
	_, err := EncodeDatagram(token, protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 1,
		Message:  protocol.ClientMoveInput{},
	}, codec)
	if !errors.Is(err, ErrDatagramTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

type largeCodec struct{ size int }

func (c largeCodec) Marshal(protocol.Message) ([]byte, error) {
	return []byte(strings.Repeat("x", c.size)), nil
}

func (largeCodec) Unmarshal(protocol.MessageType, []byte) (protocol.Message, error) {
	return protocol.ClientMoveInput{}, nil
}
