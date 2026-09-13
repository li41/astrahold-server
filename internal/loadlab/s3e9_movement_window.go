package loadlab

import (
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	s3e9MixedDynamicUpdates = parseS3E9MixedDynamicUpdates(os.Getenv("ASTRAHOLD_S3E9_MIXED_DYNAMIC_UPDATES"))
	s3e9MixedStartedUnixNS atomic.Int64
	s3e9MixedReadyClients  atomic.Int64
)

func parseS3E9MixedDynamicUpdates(value string) uint64 {
	if value == "" {
		return 0
	}
	updates, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return updates
}

func resetS3E9MixedMovementClock() {
	s3e9MixedStartedUnixNS.Store(0)
	s3e9MixedReadyClients.Store(0)
}

func s3e9MixedMovementClock() (time.Duration, int, bool) {
	startedNS := s3e9MixedStartedUnixNS.Load()
	clients := int(s3e9MixedReadyClients.Load())
	if startedNS <= 0 || clients <= 0 {
		return 0, 0, false
	}
	elapsed := time.Since(time.Unix(0, startedNS))
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed, clients, true
}

// s3e9MixedMovementWindowOpen 使用正式 WorldDynamicState delivery 數定義 active phase，
// 不依賴固定 warm-up / stop sleep。每個 ready bot先收到一筆 bootstrap dynamic state；
// 第一個 dynamic revision完成任一 delivery後開始 movement，最後一個預期 dynamic revision
// 全部 fan-out完成後關閉 movement，接著讓既有 semantic convergence tracker判斷 drain。
// 第一個 open observation 同時記錄這次 run 的 ready population 與共同 movement epoch，
// 讓所有 bot 從同一個 inward pulse 開始，而不是用各自 connection elapsed 猜 phase。
func s3e9MixedMovementWindowOpen(dynamicStates, ready uint64) bool {
	if dynamicStates == 0 {
		resetS3E9MixedMovementClock()
	}
	if !s3e9MixedMovementEnabled || ready == 0 || dynamicStates <= ready {
		return false
	}
	if s3e9MixedDynamicUpdates != 0 {
		finalStates := ready * (s3e9MixedDynamicUpdates + 1)
		if dynamicStates >= finalStates {
			return false
		}
	}
	if s3e9MixedReadyClients.Load() == 0 {
		s3e9MixedReadyClients.CompareAndSwap(0, int64(ready))
	}
	if s3e9MixedStartedUnixNS.Load() == 0 {
		s3e9MixedStartedUnixNS.CompareAndSwap(0, time.Now().UnixNano())
	}
	return true
}
