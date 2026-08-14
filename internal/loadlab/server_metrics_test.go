package loadlab

import (
	"math"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestServerCollectorSummarizesTickAOIAndReplication(t *testing.T) {
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
			SnapshotCandidates:      10,
			SnapshotTransforms:      6,
			SnapshotDeferred:        4,
			SnapshotForcedRefreshes: 1,
			SnapshotNearTransforms:  3,
			SnapshotMidTransforms:   2,
			SnapshotFarTransforms:   1,
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
	if report.Replication.SessionsReplicated != 3 || report.Replication.OutboundMessages != 9 {
		t.Fatalf("unexpected replication message report: %+v", report.Replication)
	}
	if report.Replication.SnapshotCandidates != 10 || report.Replication.SnapshotTransforms != 6 || report.Replication.SnapshotDeferred != 4 {
		t.Fatalf("unexpected snapshot replication report: %+v", report.Replication)
	}
	if report.Replication.SnapshotForcedRefreshes != 1 || report.Replication.SnapshotNearTransforms != 3 || report.Replication.SnapshotMidTransforms != 2 || report.Replication.SnapshotFarTransforms != 1 {
		t.Fatalf("unexpected tier replication report: %+v", report.Replication)
	}
	if math.Abs(report.Replication.TransformsPerSession-2) > 0.0001 {
		t.Fatalf("transforms/session=%f want=2", report.Replication.TransformsPerSession)
	}
	if math.Abs(report.Replication.DeferredCandidateRatio-0.4) > 0.0001 {
		t.Fatalf("deferred ratio=%f want=0.4", report.Replication.DeferredCandidateRatio)
	}
	if report.Errors.BlockedMoves != 1 {
		t.Fatalf("blocked moves = %d, want 1", report.Errors.BlockedMoves)
	}
	if report.TickDuration.P99MS < 5.9 || report.TickDuration.P99MS > 6.1 {
		t.Fatalf("p99 = %fms, want about 6ms", report.TickDuration.P99MS)
	}
}
