package tcpudp

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestRealtimeMailboxKeepsCorrectionSeparateFromSnapshot(t *testing.T) {
	mailbox := newRealtimeMailbox()
	done := make(chan struct{})
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
	if err := mailbox.Put(snapshot); err != nil { t.Fatal(err) }
	if err := mailbox.Put(correction); err != nil { t.Fatal(err) }

	first, ok := mailbox.Next(done)
	if !ok || first.Message.Type() != protocol.MessagePositionCorrection {
		t.Fatalf("first realtime message = %#v", first.Message)
	}
	second, ok := mailbox.Next(done)
	if !ok || second.Message.Type() != protocol.MessageWorldSnapshot {
		t.Fatalf("second realtime message = %#v", second.Message)
	}
}

func TestRealtimeMailboxNewSnapshotReplacesPendingOldSet(t *testing.T) {
	mailbox := newRealtimeMailbox()
	done := make(chan struct{})
	old0 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 1, Message: protocol.WorldSnapshot{Tick: 10, ChunkIndex: 0, ChunkCount: 3}}
	old1 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 2, Message: protocol.WorldSnapshot{Tick: 10, ChunkIndex: 1, ChunkCount: 3}}
	new0 := protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 3, Message: protocol.WorldSnapshot{Tick: 12, ChunkIndex: 0, ChunkCount: 1}}
	if err := mailbox.Put(old0); err != nil { t.Fatal(err) }
	if err := mailbox.Put(old1); err != nil { t.Fatal(err) }
	if err := mailbox.Put(new0); err != nil { t.Fatal(err) }

	got, ok := mailbox.Next(done)
	if !ok { t.Fatal("mailbox closed") }
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
	if err := mailbox.Put(bad); err != ErrRealtimeSnapshotOrder {
		t.Fatalf("expected ErrRealtimeSnapshotOrder, got %v", err)
	}
}
