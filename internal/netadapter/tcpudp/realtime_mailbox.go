package tcpudp

import (
	"errors"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
)

const maxRealtimeSnapshotChunks = 64

var (
	ErrUnsupportedRealtimeOutbound = errors.New("tcpudp: unsupported realtime outbound message")
	ErrRealtimeSnapshotOrder       = errors.New("tcpudp: realtime snapshot chunk order mismatch")
	ErrRealtimeSnapshotTooLarge    = errors.New("tcpudp: realtime snapshot has too many chunks")
)

type realtimePacketSlot struct {
	data           [MaxDatagramSize]byte
	length         int
	messageType    protocol.MessageType
	encodeDuration time.Duration
}

// realtimeMailbox 將不同 Realtime semantic stream 分開 coalesce，並在 PutEncoded 成功返回前
// 把 ASTU + ASTR + payload materialize 到 connection-owned storage：
//
//   - PositionCorrection：只保留最新 self correction。
//   - WorldSnapshot：以 tick 為 snapshot set；新 tick 的 chunk 0 會取代尚未送完的舊 set。
//
// NextPacket 會把 connection-owned slot 複製到 writer-owned reusable MTU buffer 後才解鎖，
// 因此 producer 可以在 TrySend 返回後立即重用原始 WorldSnapshot backing slice，而 writer
// 也不會在 mailbox slot 被下一 tick 覆寫時讀到破損資料。
type realtimeMailbox struct {
	mu sync.Mutex

	correction        realtimePacketSlot
	correctionPending bool

	snapshotTick     uint64
	snapshotExpected int
	snapshotReceived int
	snapshotSent     int
	snapshotChunks   []realtimePacketSlot

	wake chan struct{}
}

func newRealtimeMailbox() *realtimeMailbox {
	return &realtimeMailbox{wake: make(chan struct{}, 1)}
}

// PutEncoded 在成功返回前即取得 realtime message ownership。
// caller 傳入的 message backing storage 在此之後不再被 mailbox / writer 引用。
func (m *realtimeMailbox) PutEncoded(token Token, envelope protocol.Envelope, codec transport.PayloadCodec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch message := envelope.Message.(type) {
	case protocol.PositionCorrection:
		if err := encodeRealtimeSlot(&m.correction, token, envelope, codec); err != nil {
			return err
		}
		m.correctionPending = true
	case *protocol.PositionCorrection:
		if message == nil {
			return ErrUnsupportedRealtimeOutbound
		}
		if err := encodeRealtimeSlot(&m.correction, token, envelope, codec); err != nil {
			return err
		}
		m.correctionPending = true
	case protocol.WorldSnapshot:
		if err := m.putSnapshot(token, envelope, message, codec); err != nil {
			return err
		}
	case *protocol.WorldSnapshot:
		if message == nil {
			return ErrUnsupportedRealtimeOutbound
		}
		if err := m.putSnapshot(token, envelope, *message, codec); err != nil {
			return err
		}
	default:
		return ErrUnsupportedRealtimeOutbound
	}

	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *realtimeMailbox) putSnapshot(token Token, envelope protocol.Envelope, snapshot protocol.WorldSnapshot, codec transport.PayloadCodec) error {
	if !snapshot.ValidChunk() {
		return ErrRealtimeSnapshotOrder
	}
	if int(snapshot.ChunkCount) > maxRealtimeSnapshotChunks {
		return ErrRealtimeSnapshotTooLarge
	}

	if snapshot.ChunkIndex == 0 {
		m.snapshotTick = snapshot.Tick
		m.snapshotExpected = int(snapshot.ChunkCount)
		m.snapshotReceived = 0
		m.snapshotSent = 0
		m.ensureSnapshotSlots(m.snapshotExpected)
	} else if snapshot.Tick != m.snapshotTick || int(snapshot.ChunkCount) != m.snapshotExpected || int(snapshot.ChunkIndex) != m.snapshotReceived {
		return ErrRealtimeSnapshotOrder
	}
	if int(snapshot.ChunkIndex) != m.snapshotReceived || m.snapshotReceived >= len(m.snapshotChunks) {
		return ErrRealtimeSnapshotOrder
	}

	slot := &m.snapshotChunks[m.snapshotReceived]
	if err := encodeRealtimeSlot(slot, token, envelope, codec); err != nil {
		if snapshot.ChunkIndex == 0 {
			m.resetSnapshotSet()
		}
		return err
	}
	m.snapshotReceived++
	return nil
}

func (m *realtimeMailbox) ensureSnapshotSlots(count int) {
	if cap(m.snapshotChunks) < count {
		m.snapshotChunks = make([]realtimePacketSlot, count)
		return
	}
	m.snapshotChunks = m.snapshotChunks[:count]
}

func (m *realtimeMailbox) resetSnapshotSet() {
	m.snapshotTick = 0
	m.snapshotExpected = 0
	m.snapshotReceived = 0
	m.snapshotSent = 0
}

func encodeRealtimeSlot(slot *realtimePacketSlot, token Token, envelope protocol.Envelope, codec transport.PayloadCodec) error {
	started := time.Now()
	packet, err := AppendEncodeDatagram(slot.data[:0], token, envelope, codec)
	if err != nil {
		return err
	}
	slot.length = len(packet)
	slot.messageType = envelope.Message.Type()
	slot.encodeDuration = time.Since(started)
	return nil
}

// NextPacket 會優先取最新 correction，再依序取目前 snapshot set 的 chunk。
// packet bytes 在 lock 內複製到 writer-owned dst，所以返回後 mailbox slot 可立即被 producer 重用。
func (m *realtimeMailbox) NextPacket(dst []byte, done <-chan struct{}) ([]byte, protocol.MessageType, time.Duration, bool) {
	for {
		m.mu.Lock()
		if m.correctionPending {
			packet := copyRealtimePacket(dst, &m.correction)
			messageType := m.correction.messageType
			encodeDuration := m.correction.encodeDuration
			m.correctionPending = false
			m.mu.Unlock()
			return packet, messageType, encodeDuration, true
		}
		if m.snapshotSent < m.snapshotReceived {
			slot := &m.snapshotChunks[m.snapshotSent]
			packet := copyRealtimePacket(dst, slot)
			messageType := slot.messageType
			encodeDuration := slot.encodeDuration
			m.snapshotSent++
			if m.snapshotExpected > 0 && m.snapshotSent == m.snapshotExpected {
				m.resetSnapshotSet()
			}
			m.mu.Unlock()
			return packet, messageType, encodeDuration, true
		}
		m.mu.Unlock()

		select {
		case <-done:
			return dst[:0], protocol.MessageUnknown, 0, false
		case <-m.wake:
		}
	}
}

func copyRealtimePacket(dst []byte, slot *realtimePacketSlot) []byte {
	return append(dst[:0], slot.data[:slot.length]...)
}
