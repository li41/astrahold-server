package tcpudp

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDatagramRoundTripDoesNotExposeBearerToken(t *testing.T) {
	token := Token{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	codec := jsonv1.Codec{}
	want := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 9,
		Message: protocol.ClientMoveInput{
			DirectionX: 1,
			DirectionZ: -0.5,
		},
	}
	packet, err := EncodeClientDatagram(token, want, codec)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(packet, token[:]) {
		t.Fatal("ASTU v9 leaked the bearer realtime token on UDP wire")
	}
	route, err := ParseDatagramRoute(packet)
	if err != nil {
		t.Fatal(err)
	}
	if route != token.RoutingID() {
		t.Fatalf("route=%x want=%x", route, token.RoutingID())
	}
	got, err := DecodeClientDatagram(token, packet, codec)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sequence != 9 || got.Delivery != protocol.DeliveryRealtimeSequenced {
		t.Fatalf("header mismatch: %+v", got)
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

func TestDatagramAuthenticationRejectsTamperWrongTokenAndReflection(t *testing.T) {
	token := Token{1, 3, 5, 7, 9, 11, 13, 15, 2, 4, 6, 8, 10, 12, 14, 16}
	wrong := Token{99}
	codec := gamev1.Codec{}
	envelope := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 7,
		Message:  protocol.ClientMoveInput{DirectionX: 0.25, DirectionZ: 0.5},
	}
	packet, err := EncodeClientDatagram(token, envelope, codec)
	if err != nil {
		t.Fatal(err)
	}

	tampered := append([]byte(nil), packet...)
	tampered[len(tampered)-DatagramAuthTagSize-1] ^= 0x01
	if _, err := DecodeClientDatagram(token, tampered, codec); !errors.Is(err, ErrDatagramAuthentication) {
		t.Fatalf("tamper err=%v", err)
	}
	if _, err := DecodeClientDatagram(wrong, packet, codec); !errors.Is(err, ErrDatagramAuthentication) {
		t.Fatalf("wrong token err=%v", err)
	}

	serverPacket, err := EncodeServerDatagram(token, envelope, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeClientDatagram(token, serverPacket, codec); !errors.Is(err, ErrDatagramAuthentication) {
		t.Fatalf("S2C reflection err=%v", err)
	}
	if _, err := DecodeServerDatagram(token, serverPacket, codec); err != nil {
		t.Fatalf("valid S2C packet rejected: %v", err)
	}
}

func TestMaxSnapshotDatagramStillFitsMTUExactly(t *testing.T) {
	entities := make([]protocol.EntityTransform, protocol.MaxSnapshotEntitiesPerChunk)
	for i := range entities {
		entities[i] = protocol.EntityTransform{
			EntityID: world.EntityID(i + 1),
			Tick:     77,
			Position: world.Position{X: float32(i), Z: float32(i), Layer: 0},
		}
	}
	packet, err := EncodeServerDatagram(Token{1}, protocol.Envelope{
		Delivery:   protocol.DeliveryRealtimeSequenced,
		Sequence:   1,
		ServerTick: 77,
		Message: protocol.WorldSnapshot{
			Tick:       77,
			ChunkIndex: 0,
			ChunkCount: 1,
			Entities:   entities,
		},
	}, gamev1.Codec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != MaxDatagramSize {
		t.Fatalf("max snapshot datagram=%d want=%d", len(packet), MaxDatagramSize)
	}
}

func TestDatagramRejectsOversizePayload(t *testing.T) {
	token, _ := NewToken()
	codec := largeCodec{size: MaxDatagramSize}
	_, err := EncodeClientDatagram(token, protocol.Envelope{
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
