package tcpudp

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
