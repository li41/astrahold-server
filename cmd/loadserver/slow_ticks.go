package main

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const defaultSlowTickLimit = 8

type slowTickRecord struct {
	Tick                       uint64  `json:"tick"`
	TotalMS                    float64 `json:"total_ms"`
	CommandMS                  float64 `json:"command_ms"`
	SimulationMS               float64 `json:"simulation_ms"`
	DynamicReplicationMS       float64 `json:"dynamic_replication_ms"`
	ReplicationFrameBuildMS    float64 `json:"replication_frame_build_ms"`
	AOIMS                      float64 `json:"aoi_ms"`
	ReplicationBuildMS         float64 `json:"replication_build_ms"`
	DeliveryMS                 float64 `json:"delivery_ms"`
	VitalsReplicationMS        float64 `json:"vitals_replication_ms"`
	QueueDepthBefore           int     `json:"queue_depth_before"`
	QueueDepthAfter            int     `json:"queue_depth_after"`
	CommandsDrained            int     `json:"commands_drained"`
	SessionsReplicated         int     `json:"sessions_replicated"`
	OutboundMessages           int     `json:"outbound_messages"`
	SnapshotCandidates         int     `json:"snapshot_candidates"`
	SnapshotTransforms         int     `json:"snapshot_transforms"`
	SnapshotDeferred           int     `json:"snapshot_deferred"`
	SpawnCandidates            int     `json:"spawn_candidates"`
	SpawnSelected              int     `json:"spawn_selected"`
	SpawnDeferred              int     `json:"spawn_deferred"`
	DespawnCandidates          int     `json:"despawn_candidates"`
	DespawnSelected            int     `json:"despawn_selected"`
	DespawnDeferred            int     `json:"despawn_deferred"`
	LifecycleBackpressureStops int     `json:"lifecycle_backpressure_stops"`
}

type slowTickReport struct {
	Ticks []slowTickRecord `json:"ticks"`
}

type slowTickCollector struct {
	limit  int
	active bool
	ticks  []slowTickRecord
}

func newSlowTickCollector(limit int) *slowTickCollector {
	if limit <= 0 {
		limit = defaultSlowTickLimit
	}
	return &slowTickCollector{limit: limit, ticks: make([]slowTickRecord, 0, limit)}
}

func (c *slowTickCollector) Reset() {
	if c == nil {
		return
	}
	c.active = true
	c.ticks = c.ticks[:0]
}

func (c *slowTickCollector) Record(report worldruntime.StepReport) {
	if c == nil || !c.active || c.limit <= 0 {
		return
	}
	m := report.Metrics
	record := slowTickRecord{
		Tick:                       report.Tick,
		TotalMS:                    durationMS(m.TotalDuration),
		CommandMS:                  durationMS(m.CommandDuration),
		SimulationMS:               durationMS(m.SimulationDuration),
		DynamicReplicationMS:       durationMS(m.DynamicReplicationDuration),
		ReplicationFrameBuildMS:    durationMS(m.ReplicationFrameBuildDuration),
		AOIMS:                      durationMS(m.AOIDuration),
		ReplicationBuildMS:         durationMS(m.ReplicationBuildDuration),
		DeliveryMS:                 durationMS(m.DeliveryDuration),
		VitalsReplicationMS:        durationMS(m.VitalsReplicationDuration),
		QueueDepthBefore:           m.CommandQueueDepthBefore,
		QueueDepthAfter:            m.CommandQueueDepthAfter,
		CommandsDrained:            m.CommandsDrained,
		SessionsReplicated:         m.SessionsReplicated,
		OutboundMessages:           m.OutboundMessages,
		SnapshotCandidates:         m.SnapshotCandidates,
		SnapshotTransforms:         m.SnapshotTransforms,
		SnapshotDeferred:           m.SnapshotDeferred,
		SpawnCandidates:            m.SpawnCandidates,
		SpawnSelected:              m.SpawnSelected,
		SpawnDeferred:              m.SpawnDeferred,
		DespawnCandidates:          m.DespawnCandidates,
		DespawnSelected:            m.DespawnSelected,
		DespawnDeferred:            m.DespawnDeferred,
		LifecycleBackpressureStops: m.LifecycleBackpressureStops,
	}

	if len(c.ticks) < c.limit {
		c.ticks = append(c.ticks, record)
		c.promote(len(c.ticks) - 1)
		return
	}
	if record.TotalMS <= c.ticks[len(c.ticks)-1].TotalMS {
		return
	}
	c.ticks[len(c.ticks)-1] = record
	c.promote(len(c.ticks) - 1)
}

func (c *slowTickCollector) promote(index int) {
	for index > 0 && c.ticks[index].TotalMS > c.ticks[index-1].TotalMS {
		c.ticks[index], c.ticks[index-1] = c.ticks[index-1], c.ticks[index]
		index--
	}
}

func (c *slowTickCollector) Finish() slowTickReport {
	if c == nil {
		return slowTickReport{}
	}
	c.active = false
	out := make([]slowTickRecord, len(c.ticks))
	copy(out, c.ticks)
	return slowTickReport{Ticks: out}
}

func slowTickReportPath(reportPath string) string {
	ext := filepath.Ext(reportPath)
	base := strings.TrimSuffix(reportPath, ext)
	if base == "" {
		base = reportPath
	}
	return base + "-slow-ticks.json"
}

func durationMS(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func writeSlowTickReport(path string, report slowTickReport) error {
	return loadlab.WriteReport(path, report)
}
