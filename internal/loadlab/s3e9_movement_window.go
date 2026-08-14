package loadlab

import (
	"os"
	"strconv"
)

var s3e9MixedDynamicUpdates = parseS3E9MixedDynamicUpdates(os.Getenv("ASTRAHOLD_S3E9_MIXED_DYNAMIC_UPDATES"))

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

// s3e9MixedMovementWindowOpen 使用正式 WorldDynamicState delivery 數定義 active phase，
// 不依賴固定 warm-up / stop sleep。每個 ready bot先收到一筆 bootstrap dynamic state；
// 第一個 objective revision完成任一 delivery後開始 movement，最後一個預期 objective revision
// 全部 fan-out完成後關閉 movement，接著讓既有 semantic convergence tracker判斷 drain。
func s3e9MixedMovementWindowOpen(dynamicStates, ready uint64) bool {
	if !s3e9MixedMovementEnabled || ready == 0 || dynamicStates <= ready {
		return false
	}
	if s3e9MixedDynamicUpdates == 0 {
		return true
	}
	finalStates := ready * (s3e9MixedDynamicUpdates + 1)
	return dynamicStates < finalStates
}
