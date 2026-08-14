package tcpudp

import (
	"testing"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRealtimeMailboxKeepsCorrectionSeparateFromSnapshot(t *testing.T) {
	mailbox := newRealtimeMailbox()
	done := make(chan struct{})
	codec := gamev1.Codec{}
	snapshot := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 10,
		Message: protocol.WorldSnapshot{Tick: 50, ChunkIndex: 0, ChunkCount: 1},
	}
	correction := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 11,
		Message: protocol.PositionCorrection{Tick: 50, EntityID: 1},
	}
	if err := mailbox.PutEncoded(Token{}, snapshot, codec); err != nil { t.Fatal(err) }
	if err := mailbox.PutEncoded(Token{}, correction, codec); err != nil { t.Fatal(err) }

	first, firstType, _, ok := mailbox.NextPacket(make([]byte, 0, MaxDatagramSize), done)
	if !ok || firstType != protocol.MessagePositionCorrection {
		t.Fatalf("first realtime type=%v", firstType)
	}
	_, firstEnvelope, err := DecodeDatagram(first, codec)
	if err != nil || firstEnvelope.Message.Type() != protocol.MessagePositionCorrection {
		t.Fatalf("decode first packet: envelope=%#v err=%v", firstEnvelope, err)
	}

	second, secondType, _, ok := mailbox.NextPacket(first[:0], done)
	if !ok || secondType != protocol.MessageWorldSnapshot {
		t.Fatalf("second realtime type=%v", secondType)
	}
	_, secondEnvelope, err := DecodeDatagram(second, codec)
	if err != nil || secondEnvelope.Message.Type() != protocol.MessageWorldSnapshot {
		t.Fatalf("decode second packet: envelope=%#v err=%v", secondEnvelope, err)
	}
}

func TestRealtimeMailboxNewSnapshotReplacesPendingOldSet(t *testing.T) {
	mailbox := newRealtimeMailbox()
	done := make(chan struct{})
	codec := gamev1.Codec{}
	old0 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 1, Message: protocol.WorldSnapshot{Tick: 10, ChunkIndex: 0, ChunkCount: 3}}
	old1 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 2, Message: protocol.WorldSnapshot{Tick: 10, ChunkIndex: 1, ChunkCount: 3}}
	new0 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 3, Message: protocol.WorldSnapshot{Tick: 12, ChunkIndex: 0, ChunkCount: 1}}
	if err := mailbox.PutEncoded(Token{}, old0, codec); err != nil { t.Fatal(err) }
	if err := mailbox.PutEncoded(Token{}, old1, codec); err != nil { t.Fatal(err) }
	if err := mailbox.PutEncoded(Token{}, new0, codec); err != nil { t.Fatal(err) }

	packet, messageType, _, ok := mailbox.NextPacket(make([]byte, 0, MaxDatagramSize), done)
	if !ok || messageType != protocol.MessageWorldSnapshot { t.Fatal("mailbox closed") }
	_, got, err := DecodeDatagram(packet, codec)
	if err != nil { t.Fatal(err) }
	snapshot := got.Message.(protocol.WorldSnapshot)
	if snapshot.Tick != 12 || snapshot.ChunkIndex != 0 || snapshot.ChunkCount != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestRealtimeMailboxRejectsOutOfOrderChunk(t *testing.T) {
	mailbox := newRealtimeMailbox()
	bad := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Message: protocol.WorldSnapshot{Tick: 10, ChunkIndex: 1, ChunkCount: 2},
	}
	if err := mailbox.PutEncoded(Token{}, bad, gamev1.Codec{}); err != ErrRealtimeSnapshotOrder {
		t.Fatalf("expected ErrRealtimeSnapshotOrder, got %v", err)
	}
}

func TestRealtimeMailboxOwnsSnapshotBytesBeforePutReturns(t *testing.T) {
	mailbox := newRealtimeMailbox()
	codec := gamev1.Codec{}
	entities := []protocol.EntityTransform{{
		EntityID: 9,
		Tick:     77,
		Position: world.Position{X: 1, Y: 2, Z: 3, Layer: 4},
		Yaw:      0.5,
	}}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryRealtimeSequenced,
		Sequence:   22,
		ServerTick: 77,
		Message: protocol.WorldSnapshot{
			Tick:        77,
			ChunkIndex: 0,
			ChunkCount: 1,
			Entities:   entities,
		},
	}
	if err := mailbox.PutEncoded(Token{}, envelope, codec); err != nil { t.Fatal(err) }

	// caller 在 TrySend/PutEncoded 返回後立即覆寫原 backing storage；mailbox wire bytes 必須不受影響。
	entities[0].Position.X = 999
	entities[0].Yaw = 9

	packet, _, _, ok := mailbox.NextPacket(make([]byte, 0, MaxDatagramSize), make(chan struct{}))
	if !ok { t.Fatal("mailbox closed") }
	_, decoded, err := DecodeDatagram(packet, codec)
	if err != nil { t.Fatal(err) }
	snapshot := decoded.Message.(protocol.WorldSnapshot)
	if got := snapshot.Entities[0]; got.Position.X != 1 || got.Yaw != 0.5 {
		t.Fatalf("mailbox retained caller backing storage: %+v", got)
	}
}
