package loadlab

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestServerCollectorSummarizesTickAndAOI(t *testing.T) {
	collector := NewServerCollector(20, 10)
	collector.Reset()
	collector.RecordStep(worldruntime.StepReport{
		Tick:       1,
		TickErrors: []simulation.TickError{{Err: navigation.ErrBlocked}},
		Metrics: worldruntime.StepMetrics{
			CommandQueueDepthBefore: 7,
			CommandQueueDepthAfter:  2,
			CommandsDrained:         5,
			SessionsReplicated:      3,
			AOIQueries:              3,
			AOICandidates:           18,
			AOIVisible:              12,
			OutboundMessages:        9,
			SimulationDuration:      time.Millisecond,
			AOIDuration:             2 * time.Millisecond,
			ReplicationBuildDuration: time.Millisecond,
			DeliveryDuration:        time.Millisecond,
			TotalDuration:           6 * time.Millisecond,
		},
	})
	report := collector.Finish(ScenarioGateZerg, 3)
	if report.Ticks != 1 || report.Queue.MaxDepthBefore != 7 || report.Queue.CommandsTotal != 5 {
		t.Fatalf("unexpected queue report: %+v", report.Queue)
	}
	if report.AOI.Queries != 3 || report.AOI.Candidates != 18 || report.AOI.Visible != 12 {
		t.Fatalf("unexpected AOI report: %+v", report.AOI)
	}
	if report.Errors.BlockedMoves != 1 {
		t.Fatalf("blocked moves = %d, want 1", report.Errors.BlockedMoves)
	}
	if report.TickDuration.P99MS < 5.9 || report.TickDuration.P99MS > 6.1 {
		t.Fatalf("p99 = %fms, want about 6ms", report.TickDuration.P99MS)
	}
}
