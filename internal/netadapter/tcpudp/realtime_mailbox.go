package tcpudp

import (
	"errors"
	"sync"

	"github.com/li41/astrahold-server/internal/protocol"
)

const maxRealtimeSnapshotChunks = 64

var (
	ErrUnsupportedRealtimeOutbound = errors.New("tcpudp: unsupported realtime outbound message")
	ErrRealtimeSnapshotOrder       = errors.New("tcpudp: realtime snapshot chunk order mismatch")
	ErrRealtimeSnapshotTooLarge    = errors.New("tcpudp: realtime snapshot has too many chunks")
)

// realtimeMailbox 將不同 Realtime semantic stream 分開 coalesce：
//
//   - PositionCorrection：只保留最新 self correction。
//   - WorldSnapshot：以 tick 為 snapshot set；新 tick 的 chunk 0 會取代尚未送完的舊 set。
//
// Writer 可以在 producer 尚未放完同一 tick 所有 chunk 時開始送；只有已送達 mailbox 的 chunk
// 才會被取出。Client 端必須收齊 ChunkCount 才提交，因此 UDP loss 或被新 tick 中斷都不會套用半張 snapshot。
type realtimeMailbox struct {
	mu sync.Mutex

	correction *protocol.Envelope

	snapshotTick     uint64
	snapshotExpected int
	snapshotSent     int
	snapshotChunks   []protocol.Envelope

	wake chan struct{}
}

func newRealtimeMailbox() *realtimeMailbox {
	return &realtimeMailbox{
		snapshotChunks: make([]protocol.Envelope, 0, 16),
		wake:           make(chan struct{}, 1),
	}
}

func (m *realtimeMailbox) Put(envelope protocol.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch message := envelope.Message.(type) {
	case protocol.PositionCorrection:
		copy := envelope
		m.correction = &copy
	case *protocol.PositionCorrection:
		if message == nil {
			return ErrUnsupportedRealtimeOutbound
		}
		copy := envelope
		m.correction = &copy
	case protocol.WorldSnapshot:
		if err := m.putSnapshot(envelope, message); err != nil {
			return err
		}
	case *protocol.WorldSnapshot:
		if message == nil {
			return ErrUnsupportedRealtimeOutbound
		}
		if err := m.putSnapshot(envelope, *message); err != nil {
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

func (m *realtimeMailbox) putSnapshot(envelope protocol.Envelope, snapshot protocol.WorldSnapshot) error {
	if !snapshot.ValidChunk() {
		return ErrRealtimeSnapshotOrder
	}
	if int(snapshot.ChunkCount) > maxRealtimeSnapshotChunks {
		return ErrRealtimeSnapshotTooLarge
	}

	if snapshot.ChunkIndex == 0 {
		m.snapshotTick = snapshot.Tick
		m.snapshotExpected = int(snapshot.ChunkCount)
		m.snapshotSent = 0
		m.snapshotChunks = m.snapshotChunks[:0]
	} else {
		if snapshot.Tick != m.snapshotTick || int(snapshot.ChunkCount) != m.snapshotExpected || int(snapshot.ChunkIndex) != len(m.snapshotChunks) {
			return ErrRealtimeSnapshotOrder
		}
	}
	if int(snapshot.ChunkIndex) != len(m.snapshotChunks) {
		return ErrRealtimeSnapshotOrder
	}
	m.snapshotChunks = append(m.snapshotChunks, envelope)
	return nil
}

// Next 會優先取最新 correction，再依序取目前 snapshot set 的 chunk。
// 若 producer 尚未放入下一個 chunk，會等待 wake；若新的 chunk 0 到來，舊 set 尚未送出的部分會被直接取代。
func (m *realtimeMailbox) Next(done <-chan struct{}) (protocol.Envelope, bool) {
	for {
		m.mu.Lock()
		if m.correction != nil {
			envelope := *m.correction
			m.correction = nil
			m.mu.Unlock()
			return envelope, true
		}
		if m.snapshotSent < len(m.snapshotChunks) {
			envelope := m.snapshotChunks[m.snapshotSent]
			m.snapshotSent++
			if m.snapshotExpected > 0 && m.snapshotSent == m.snapshotExpected {
				m.snapshotTick = 0
				m.snapshotExpected = 0
				m.snapshotSent = 0
				m.snapshotChunks = m.snapshotChunks[:0]
			}
			m.mu.Unlock()
			return envelope, true
		}
		m.mu.Unlock()

		select {
		case <-done:
			return protocol.Envelope{}, false
		case <-m.wake:
		}
	}
}
