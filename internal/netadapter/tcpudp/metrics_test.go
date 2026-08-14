package tcpudp

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestNetworkCountersRecordAndReset(t *testing.T) {
	var counters networkCounters
	counters.recordRealtime(protocol.MessageWorldSnapshot, 1184, 20*time.Microsecond)
	counters.recordRealtime(protocol.MessagePositionCorrection, 90, 5*time.Microsecond)

	got := counters.snapshot()
	if got.RealtimeDatagrams != 2 || got.RealtimeBytes != 1274 {
		t.Fatalf("realtime=%+v", got)
	}
	if got.SnapshotDatagrams != 1 || got.SnapshotBytes != 1184 {
		t.Fatalf("snapshot=%+v", got)
	}
	if got.CorrectionDatagrams != 1 || got.CorrectionBytes != 90 {
		t.Fatalf("correction=%+v", got)
	}
	if got.EncodeNanoseconds != uint64(25*time.Microsecond) {
		t.Fatalf("encode ns=%d", got.EncodeNanoseconds)
	}

	counters.reset()
	if got := counters.snapshot(); got != (NetworkMetrics{}) {
		t.Fatalf("after reset=%+v", got)
	}
}
