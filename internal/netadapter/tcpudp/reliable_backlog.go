package tcpudp

// ReliableBacklog 是 Load Lab 用的 transport drain snapshot。
// Queued 只包含尚在 channel 的 Envelope；InFlight 另外計入 writer 已 dequeue 但尚未完成 TCP write 的項目。
type ReliableBacklog struct {
	ReadyPeers        int `json:"ready_peers"`
	Queued            int `json:"queued"`
	InFlight          int `json:"in_flight"`
	MaxQueuedPerPeer  int `json:"max_queued_per_peer"`
}

func (b ReliableBacklog) Drained(expectedPeers int) bool {
	return expectedPeers > 0 && b.ReadyPeers == expectedPeers && b.Queued == 0 && b.InFlight == 0
}

// ReliableBacklog 可由 Load Lab goroutine 安全呼叫；Server peer map 受 RLock 保護，
// channel len 與 reliableInFlight atomic 都不需要取得 connection writer 的內部 lock。
func (s *Server) ReliableBacklog() ReliableBacklog {
	if s == nil {
		return ReliableBacklog{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var backlog ReliableBacklog
	for _, p := range s.peers {
		if p == nil || p.conn == nil || !p.ready.Load() {
			continue
		}
		backlog.ReadyPeers++
		queued := len(p.conn.reliable)
		backlog.Queued += queued
		if queued > backlog.MaxQueuedPerPeer {
			backlog.MaxQueuedPerPeer = queued
		}
		if p.conn.reliableInFlight.Load() {
			backlog.InFlight++
		}
	}
	return backlog
}
