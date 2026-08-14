package spatial

// CellSize 回傳 spatial acceleration grid 的 cell edge length。
// replication read frame 只讀這個 immutable configuration，不讀 mutable buckets。
func (g *Grid) CellSize() float32 { return g.cellSize }
