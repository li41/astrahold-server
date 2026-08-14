package tcpudp

import (
	"sync/atomic"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
)

// NetworkMetrics 是 Siege Load Lab 可讀取的低成本 transport snapshot。
// counters 只記錄成功 encode 的 realtime datagram；write error 仍由 NetworkError gate 負責。
type NetworkMetrics struct {
	RealtimeDatagrams    uint64
	RealtimeBytes        uint64
	SnapshotDatagrams    uint64
	SnapshotBytes        uint64
	CorrectionDatagrams  uint64
	CorrectionBytes      uint64
	EncodeNanoseconds    uint64
}

type networkCounters struct {
	realtimeDatagrams   atomic.Uint64
	realtimeBytes       atomic.Uint64
	snapshotDatagrams   atomic.Uint64
	snapshotBytes       atomic.Uint64
	correctionDatagrams atomic.Uint64
	correctionBytes     atomic.Uint64
	encodeNanoseconds   atomic.Uint64
}

func (m *networkCounters) reset() {
	m.realtimeDatagrams.Store(0)
	m.realtimeBytes.Store(0)
	m.snapshotDatagrams.Store(0)
	m.snapshotBytes.Store(0)
	m.correctionDatagrams.Store(0)
	m.correctionBytes.Store(0)
	m.encodeNanoseconds.Store(0)
}

func (m *networkCounters) recordRealtime(messageType protocol.MessageType, bytes int, encodeDuration time.Duration) {
	if m == nil || bytes < 0 {
		return
	}
	m.realtimeDatagrams.Add(1)
	m.realtimeBytes.Add(uint64(bytes))
	m.encodeNanoseconds.Add(uint64(encodeDuration))
	switch messageType {
	case protocol.MessageWorldSnapshot:
		m.snapshotDatagrams.Add(1)
		m.snapshotBytes.Add(uint64(bytes))
	case protocol.MessagePositionCorrection:
		m.correctionDatagrams.Add(1)
		m.correctionBytes.Add(uint64(bytes))
	}
}

func (m *networkCounters) snapshot() NetworkMetrics {
	if m == nil {
		return NetworkMetrics{}
	}
	return NetworkMetrics{
		RealtimeDatagrams:   m.realtimeDatagrams.Load(),
		RealtimeBytes:       m.realtimeBytes.Load(),
		SnapshotDatagrams:   m.snapshotDatagrams.Load(),
		SnapshotBytes:       m.snapshotBytes.Load(),
		CorrectionDatagrams: m.correctionDatagrams.Load(),
		CorrectionBytes:     m.correctionBytes.Load(),
		EncodeNanoseconds:   m.encodeNanoseconds.Load(),
	}
}

// ReadyPeerCount 回傳已完成 Reliable bootstrap、已排入 Join 的 peer 數量。
// 主要供 Siege Load Lab 判斷何時開始正式量測；一般 gameplay 不應依賴此值。
func (s *Server) ReadyPeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, p := range s.peers {
		if p != nil && p.ready.Load() {
			count++
		}
	}
	return count
}

// ResetNetworkMetrics 將 transport counters 對齊 Load Lab measurement window。
func (s *Server) ResetNetworkMetrics() { s.metrics.reset() }

// NetworkMetrics 回傳 lock-free counters snapshot。
func (s *Server) NetworkMetrics() NetworkMetrics { return s.metrics.snapshot() }
