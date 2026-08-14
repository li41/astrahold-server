package tcpudp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAppendEncodeDatagramMatchesLegacyWireLayout(t *testing.T) {
	token := Token{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryRealtimeSequenced,
		Sequence:   42,
		ServerTick: 77,
		Message: protocol.WorldSnapshot{
			Tick:       77,
			ChunkIndex: 0,
			ChunkCount: 1,
			Entities: []protocol.EntityTransform{
				{EntityID: 10, Tick: 77, Position: world.Position{X: 1, Y: 2, Z: 3, Layer: 4}, Yaw: 0.5},
				{EntityID: 11, Tick: 77, Position: world.Position{X: 5, Y: 6, Z: 7, Layer: 8}, Yaw: 1.5},
			},
		},
	}
	codec := gamev1.Codec{}

	frame, err := transport.EncodeEnvelope(envelope, codec)
	if err != nil {
		t.Fatal(err)
	}
	legacy := make([]byte, DatagramHeaderSize+len(frame))
	binary.BigEndian.PutUint32(legacy[0:4], DatagramMagic)
	binary.BigEndian.PutUint16(legacy[4:6], protocol.Version)
	binary.BigEndian.PutUint16(legacy[6:8], DatagramHeaderSize)
	copy(legacy[8:24], token[:])
	copy(legacy[24:], frame)

	buffer := make([]byte, 0, MaxDatagramSize)
	got, err := AppendEncodeDatagram(buffer, token, envelope, codec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, legacy) {
		t.Fatalf("single-pass datagram differs from legacy wire\ngot=%x\nwant=%x", got, legacy)
	}

	decodedToken, decoded, err := DecodeDatagram(got, codec)
	if err != nil {
		t.Fatal(err)
	}
	if decodedToken != token || decoded.Sequence != envelope.Sequence || decoded.ServerTick != envelope.ServerTick {
		t.Fatalf("decoded metadata token=%v envelope=%+v", decodedToken, decoded)
	}
}

func TestAppendEncodeDatagramReusesProvidedBuffer(t *testing.T) {
	token := Token{1}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryRealtimeSequenced,
		Sequence:   1,
		ServerTick: 1,
		Message: protocol.PositionCorrection{
			Tick:     1,
			EntityID: 1,
			Position: world.Position{X: 1, Y: 2, Z: 3, Layer: 1},
			Yaw:      0.25,
		},
	}
	codec := gamev1.Codec{}
	buffer := make([]byte, 0, MaxDatagramSize)

	allocs := testing.AllocsPerRun(1000, func() {
		packet, err := AppendEncodeDatagram(buffer[:0], token, envelope, codec)
		if err != nil {
			panic(err)
		}
		if len(packet) == 0 {
			panic("empty packet")
		}
	})
	if allocs != 0 {
		t.Fatalf("allocations per reusable encode=%f want=0", allocs)
	}
}
