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
	Tick                                     uint64  `json:"tick"`
	TotalMS                                  float64 `json:"total_ms"`
	CommandMS                                float64 `json:"command_ms"`
	SimulationMS                             float64 `json:"simulation_ms"`
	DynamicReplicationMS                     float64 `json:"dynamic_replication_ms"`
	ReplicationFrameBuildMS                  float64 `json:"replication_frame_build_ms"`
	AOIMS                                    float64 `json:"aoi_ms"`
	ReplicationBuildMS                       float64 `json:"replication_build_ms"`
	DeliveryMS                               float64 `json:"delivery_ms"`
	VitalsReplicationMS                      float64 `json:"vitals_replication_ms"`
	QueueDepthBefore                         int     `json:"queue_depth_before"`
	QueueDepthAfter                          int     `json:"queue_depth_after"`
	CommandsDrained                          int     `json:"commands_drained"`
	EntityActionsApplied                     int     `json:"entity_actions_applied"`
	ActionRejections                         int     `json:"action_rejections"`
	DirtyVitalsGlobalBudget                  int     `json:"dirty_vitals_global_budget"`
	DirtyVitalsSelected                      int     `json:"dirty_vitals_selected"`
	DirtyVitalsGlobalBudgetExhausted         bool    `json:"dirty_vitals_global_budget_exhausted"`
	DirtyVitalsEntities                      int     `json:"dirty_vitals_entities"`
	DirtyVitalsOldestDirtyAgeTicks           uint64  `json:"dirty_vitals_oldest_dirty_age_ticks"`
	DirtyVitalsOldestPendingRevisionAgeTicks uint64  `json:"dirty_vitals_oldest_pending_revision_age_ticks"`
	DirtyVitalsOldestPendingEntityID         uint64  `json:"dirty_vitals_oldest_pending_entity_id"`
	DirtyVitalsOldestPendingSessionID        uint64  `json:"dirty_vitals_oldest_pending_session_id"`
	DirtyVitalsEntityCompletions             int     `json:"dirty_vitals_entity_completions"`
	DirtyVitalsMaxEntityCompletionTicks      uint64  `json:"dirty_vitals_max_entity_completion_ticks"`
	DirtyVitalsMaxRevisionCompletionTicks    uint64  `json:"dirty_vitals_max_revision_completion_ticks"`
	DirtyVitalsSessionCursorAdvances         int     `json:"dirty_vitals_session_cursor_advances"`
	DirtyVitalsSessionCursorWraps            int     `json:"dirty_vitals_session_cursor_wraps"`
	SessionsReplicated                       int     `json:"sessions_replicated"`
	OutboundMessages                         int     `json:"outbound_messages"`
	SnapshotCandidates                       int     `json:"snapshot_candidates"`
	SnapshotTransforms                       int     `json:"snapshot_transforms"`
	SnapshotDeferred                         int     `json:"snapshot_deferred"`
	SpawnCandidates                          int     `json:"spawn_candidates"`
	SpawnSelected                            int     `json:"spawn_selected"`
	SpawnDeferred                            int     `json:"spawn_deferred"`
	DespawnCandidates                        int     `json:"despawn_candidates"`
	DespawnSelected                          int     `json:"despawn_selected"`
	DespawnDeferred                          int     `json:"despawn_deferred"`
	LifecycleBackpressureStops               int     `json:"lifecycle_backpressure_stops"`
	LifecycleGlobalBudget                    int     `json:"lifecycle_global_budget"`
	LifecycleGlobalSelected                  int     `json:"lifecycle_global_selected"`
	LifecycleGlobalBudgetExhausted           bool    `json:"lifecycle_global_budget_exhausted"`
	InitialVitalsGlobalBudget                int     `json:"initial_vitals_global_budget"`
	InitialVitalsGlobalSelected              int     `json:"initial_vitals_global_selected"`
	InitialVitalsGlobalBudgetExhausted       bool    `json:"initial_vitals_global_budget_exhausted"`
}

type dirtyVitalsFairnessMetrics struct {
	MaxDirtyEntities                      int    `json:"max_dirty_entities"`
	OldestDirtyAgeTicks                   uint64 `json:"oldest_dirty_age_ticks"`
	OldestPendingRevisionAgeTicks         uint64 `json:"oldest_pending_revision_age_ticks"`
	OldestPendingEntityID                 uint64 `json:"oldest_pending_entity_id"`
	OldestPendingSessionID                uint64 `json:"oldest_pending_session_id"`
	EntityCompletions                     uint64 `json:"entity_completions"`
	MaxEntityCompletionTicks              uint64 `json:"max_entity_completion_ticks"`
	MaxRevisionCompletionTicks            uint64 `json:"max_revision_completion_ticks"`
	BudgetExhaustions                     uint64 `json:"budget_exhaustions"`
	SessionCursorAdvances                 uint64 `json:"session_cursor_advances"`
	SessionCursorWraps                    uint64 `json:"session_cursor_wraps"`
}

type slowTickReport struct {
	Ticks    []slowTickRecord            `json:"ticks"`
	Combat   churnCombatMetrics          `json:"combat"`
	Fairness dirtyVitalsFairnessMetrics  `json:"dirty_vitals_fairness"`
}

type slowTickCollector struct {
	limit    int
	active   bool
	ticks    []slowTickRecord
	combat   churnCombatMetrics
	fairness dirtyVitalsFairnessMetrics
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
	c.combat = churnCombatMetrics{}
	c.fairness = dirtyVitalsFairnessMetrics{}
}

func (c *slowTickCollector) Record(report worldruntime.StepReport) {
	if c == nil || !c.active {
		return
	}
	m := report.Metrics
	c.combat.ActionsApplied += uint64(m.EntityActionsApplied)
	c.combat.DirtyVitalsSelected += uint64(m.DirtyVitalsSelected)
	c.combat.ActionRejections += uint64(len(report.ActionRejections))
	if m.DirtyVitalsSelected > c.combat.MaxDirtyVitalsSelectedPerTick {
		c.combat.MaxDirtyVitalsSelectedPerTick = m.DirtyVitalsSelected
	}
	if m.DirtyVitalsEntities > c.fairness.MaxDirtyEntities {
		c.fairness.MaxDirtyEntities = m.DirtyVitalsEntities
	}
	if m.DirtyVitalsOldestDirtyAgeTicks > c.fairness.OldestDirtyAgeTicks {
		c.fairness.OldestDirtyAgeTicks = m.DirtyVitalsOldestDirtyAgeTicks
	}
	if m.DirtyVitalsOldestPendingRevisionAgeTicks > c.fairness.OldestPendingRevisionAgeTicks || (c.fairness.OldestPendingEntityID == 0 && m.DirtyVitalsOldestPendingEntityID != 0) {
		c.fairness.OldestPendingRevisionAgeTicks = m.DirtyVitalsOldestPendingRevisionAgeTicks
		c.fairness.OldestPendingEntityID = uint64(m.DirtyVitalsOldestPendingEntityID)
		c.fairness.OldestPendingSessionID = uint64(m.DirtyVitalsOldestPendingSessionID)
	}
	c.fairness.EntityCompletions += uint64(m.DirtyVitalsEntityCompletions)
	if m.DirtyVitalsMaxEntityCompletionTicks > c.fairness.MaxEntityCompletionTicks {
		c.fairness.MaxEntityCompletionTicks = m.DirtyVitalsMaxEntityCompletionTicks
	}
	if m.DirtyVitalsMaxRevisionCompletionTicks > c.fairness.MaxRevisionCompletionTicks {
		c.fairness.MaxRevisionCompletionTicks = m.DirtyVitalsMaxRevisionCompletionTicks
	}
	if m.DirtyVitalsGlobalBudgetExhausted {
		c.fairness.BudgetExhaustions++
	}
	c.fairness.SessionCursorAdvances += uint64(m.DirtyVitalsSessionCursorAdvances)
	c.fairness.SessionCursorWraps += uint64(m.DirtyVitalsSessionCursorWraps)
	if c.limit <= 0 {
		return
	}
	record := slowTickRecord{
		Tick:                                     report.Tick,
		TotalMS:                                  durationMS(m.TotalDuration),
		CommandMS:                                durationMS(m.CommandDuration),
		SimulationMS:                             durationMS(m.SimulationDuration),
		DynamicReplicationMS:                     durationMS(m.DynamicReplicationDuration),
		ReplicationFrameBuildMS:                  durationMS(m.ReplicationFrameBuildDuration),
		AOIMS:                                    durationMS(m.AOIDuration),
		ReplicationBuildMS:                       durationMS(m.ReplicationBuildDuration),
		DeliveryMS:                               durationMS(m.DeliveryDuration),
		VitalsReplicationMS:                      durationMS(m.VitalsReplicationDuration),
		QueueDepthBefore:                         m.CommandQueueDepthBefore,
		QueueDepthAfter:                          m.CommandQueueDepthAfter,
		CommandsDrained:                          m.CommandsDrained,
		EntityActionsApplied:                     m.EntityActionsApplied,
		ActionRejections:                         len(report.ActionRejections),
		DirtyVitalsGlobalBudget:                  m.DirtyVitalsGlobalBudget,
		DirtyVitalsSelected:                      m.DirtyVitalsSelected,
		DirtyVitalsGlobalBudgetExhausted:         m.DirtyVitalsGlobalBudgetExhausted,
		DirtyVitalsEntities:                      m.DirtyVitalsEntities,
		DirtyVitalsOldestDirtyAgeTicks:           m.DirtyVitalsOldestDirtyAgeTicks,
		DirtyVitalsOldestPendingRevisionAgeTicks: m.DirtyVitalsOldestPendingRevisionAgeTicks,
		DirtyVitalsOldestPendingEntityID:         uint64(m.DirtyVitalsOldestPendingEntityID),
		DirtyVitalsOldestPendingSessionID:        uint64(m.DirtyVitalsOldestPendingSessionID),
		DirtyVitalsEntityCompletions:             m.DirtyVitalsEntityCompletions,
		DirtyVitalsMaxEntityCompletionTicks:      m.DirtyVitalsMaxEntityCompletionTicks,
		DirtyVitalsMaxRevisionCompletionTicks:    m.DirtyVitalsMaxRevisionCompletionTicks,
		DirtyVitalsSessionCursorAdvances:         m.DirtyVitalsSessionCursorAdvances,
		DirtyVitalsSessionCursorWraps:            m.DirtyVitalsSessionCursorWraps,
		SessionsReplicated:                       m.SessionsReplicated,
		OutboundMessages:                         m.OutboundMessages,
		SnapshotCandidates:                       m.SnapshotCandidates,
		SnapshotTransforms:                       m.SnapshotTransforms,
		SnapshotDeferred:                         m.SnapshotDeferred,
		SpawnCandidates:                          m.SpawnCandidates,
		SpawnSelected:                            m.SpawnSelected,
		SpawnDeferred:                            m.SpawnDeferred,
		DespawnCandidates:                        m.DespawnCandidates,
		DespawnSelected:                          m.DespawnSelected,
		DespawnDeferred:                          m.DespawnDeferred,
		LifecycleBackpressureStops:               m.LifecycleBackpressureStops,
		LifecycleGlobalBudget:                    m.LifecycleGlobalBudget,
		LifecycleGlobalSelected:                  m.LifecycleGlobalSelected,
		LifecycleGlobalBudgetExhausted:           m.LifecycleGlobalBudgetExhausted,
		InitialVitalsGlobalBudget:                m.InitialVitalsGlobalBudget,
		InitialVitalsGlobalSelected:              m.InitialVitalsGlobalSelected,
		InitialVitalsGlobalBudgetExhausted:       m.InitialVitalsGlobalBudgetExhausted,
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
	return slowTickReport{Ticks: out, Combat: c.combat, Fairness: c.fairness}
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
