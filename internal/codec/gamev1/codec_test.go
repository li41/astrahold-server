package gamev1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRealtimeMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	messages := []protocol.Message{
		protocol.ClientMoveInput{DirectionX: 0.25, DirectionZ: -0.75},
		protocol.WorldSnapshot{
			Tick: 90, ChunkIndex: 1, ChunkCount: 3,
			Entities: []protocol.EntityTransform{{
				EntityID: 7, Tick: 90,
				Position: world.Position{X: 1.25, Y: 8, Z: -4.5, Layer: 2},
				Yaw: 1.5,
			}},
		},
		protocol.PositionCorrection{
			Tick: 90, EntityID: 7,
			Position: world.Position{X: 1.25, Y: 8, Z: -4.5, Layer: 2},
			Yaw: 1.5, LastProcessedInputSequence: 123,
		},
	}
	for _, message := range messages {
		payload, err := codec.Marshal(message)
		if err != nil {
			t.Fatalf("marshal %T: %v", message, err)
		}
		decoded, err := codec.Unmarshal(message.Type(), payload)
		if err != nil {
			t.Fatalf("unmarshal %T: %v", message, err)
		}
		if decoded.Type() != message.Type() {
			t.Fatalf("type mismatch got=%d want=%d", decoded.Type(), message.Type())
		}
	}
}

func TestMaxSnapshotChunkFitsDatagramGuard(t *testing.T) {
	entities := make([]protocol.EntityTransform, protocol.MaxSnapshotEntitiesPerChunk)
	for i := range entities {
		entities[i] = protocol.EntityTransform{
			EntityID: world.EntityID(i + 1), Tick: 1,
			Position: world.Position{X: float32(i), Y: 8, Z: float32(-i), Layer: 2},
			Yaw: 0.5,
		}
	}
	envelope := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 1,
		ServerTick: 1,
		Message: protocol.WorldSnapshot{Tick: 1, ChunkIndex: 0, ChunkCount: 1, Entities: entities},
	}
	packet, err := tcpudp.EncodeDatagram(tcpudp.Token{}, envelope, Codec{})
	if err != nil {
		t.Fatalf("EncodeDatagram: %v", err)
	}
	if len(packet) > tcpudp.MaxDatagramSize {
		t.Fatalf("snapshot datagram too large: %d > %d", len(packet), tcpudp.MaxDatagramSize)
	}
	if len(packet) != 1184 {
		t.Fatalf("unexpected max snapshot datagram size: %d", len(packet))
	}
}

func TestSnapshotRejectsOversizedChunk(t *testing.T) {
	codec := Codec{}
	message := protocol.WorldSnapshot{
		Tick: 1, ChunkIndex: 0, ChunkCount: 1,
		Entities: make([]protocol.EntityTransform, protocol.MaxSnapshotEntitiesPerChunk+1),
	}
	if _, err := codec.Marshal(message); err != ErrInvalidSnapshotChunk {
		t.Fatalf("expected ErrInvalidSnapshotChunk, got %v", err)
	}
}
