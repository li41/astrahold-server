package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type churnSoakRoundSummary struct {
	Round                       int     `json:"round"`
	Direction                   string  `json:"direction"`
	TriggerToConvergedSeconds   float64 `json:"trigger_to_converged_seconds"`
	ObservedNonConverged        bool    `json:"observed_non_converged"`
	DesiredRelationships        int     `json:"desired_relationships"`
	KnownDesired                int     `json:"known_desired"`
	PendingSpawns               int     `json:"pending_spawns"`
	PendingDespawns             int     `json:"pending_despawns"`
	PendingVitalsEntities       int     `json:"pending_vitals_entities"`
	PendingDynamicSessions      int     `json:"pending_dynamic_sessions"`
	ReliableQueued              int     `json:"reliable_queued"`
	ReliableInFlight            int     `json:"reliable_in_flight"`
	TickP99MS                   float64 `json:"tick_p99_ms"`
	TickMaxMS                   float64 `json:"tick_max_ms"`
	SpawnSelected               uint64  `json:"spawn_selected"`
	DespawnSelected             uint64  `json:"despawn_selected"`
	InitialVitalsSelected       uint64  `json:"initial_vitals_selected"`
	LifecycleBackpressureStops  uint64  `json:"lifecycle_backpressure_stops"`
	MaxLifecycleSelectedPerTick int     `json:"max_lifecycle_selected_per_tick"`
	MaxInitialVitalsPerTick     int     `json:"max_initial_vitals_selected_per_tick"`
	TotalAllocBytes             uint64  `json:"total_alloc_bytes"`
	Mallocs                     uint64  `json:"mallocs"`
	HeapAllocBytes              uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes                uint64  `json:"heap_sys_bytes"`
	NumGC                       uint32  `json:"num_gc"`
	GCPauseTotalMS              float64 `json:"gc_pause_total_ms"`
	CommandErrors               uint64  `json:"command_errors"`
	UnexpectedTickErrors        uint64  `json:"unexpected_tick_errors"`
	DeliveryErrors              uint64  `json:"delivery_errors"`
	NetworkErrors               uint64  `json:"network_errors"`
	DatagramTooLarge            uint64  `json:"datagram_too_large"`
}

type churnSoakSummary struct {
	SchemaVersion                int                     `json:"schema_version"`
	Scenario                     loadlab.Scenario        `json:"scenario"`
	ExpectedClients              int                     `json:"expected_clients"`
	Rounds                       int                     `json:"rounds"`
	Round                        []churnSoakRoundSummary `json:"round"`
	MaxTriggerToConvergedSeconds float64                 `json:"max_trigger_to_converged_seconds"`
	MaxTickP99MS                 float64                 `json:"max_tick_p99_ms"`
	MaxTickMS                    float64                 `json:"max_tick_ms"`
	TotalSpawnSelected           uint64                  `json:"total_spawn_selected"`
	TotalDespawnSelected         uint64                  `json:"total_despawn_selected"`
	TotalInitialVitalsSelected   uint64                  `json:"total_initial_vitals_selected"`
	TotalAllocBytes              uint64                  `json:"total_alloc_bytes"`
	MaxRoundTotalAllocBytes      uint64                  `json:"max_round_total_alloc_bytes"`
	MaxHeapAllocBytes            uint64                  `json:"max_heap_alloc_bytes"`
	MaxHeapSysBytes              uint64                  `json:"max_heap_sys_bytes"`
	FirstRoundHeapAllocBytes     uint64                  `json:"first_round_heap_alloc_bytes"`
	LastRoundHeapAllocBytes      uint64                  `json:"last_round_heap_alloc_bytes"`
	HeapAllocGrowthBytes         int64                   `json:"heap_alloc_growth_bytes"`
	FirstRoundHeapSysBytes       uint64                  `json:"first_round_heap_sys_bytes"`
	LastRoundHeapSysBytes        uint64                  `json:"last_round_heap_sys_bytes"`
	HeapSysGrowthBytes           int64                   `json:"heap_sys_growth_bytes"`
	TotalGC                      uint64                  `json:"total_gc"`
	TotalGCPauseMS               float64                 `json:"total_gc_pause_ms"`
	TotalLifecycleBackpressure   uint64                  `json:"total_lifecycle_backpressure_stops"`
	MaxLifecycleSelectedPerTick  int                     `json:"max_lifecycle_selected_per_tick"`
	MaxInitialVitalsSelectedTick int                     `json:"max_initial_vitals_selected_per_tick"`
}

func runTeleportChurnRounds(
	ctx context.Context,
	rounds int,
	swapRequests, restoreRequests []worldruntime.TeleportRequest,
	worldRuntime *worldruntime.Runtime,
	collector *loadlab.ServerCollector,
	slowTicks *slowTickCollector,
	convergence *convergenceTracker,
	server *tcpudp.Server,
	scenario loadlab.Scenario,
	clients int,
	timeout, stableFor time.Duration,
	reportPath string,
) (convergenceMetadata, error) {
	if rounds <= 0 || len(swapRequests) == 0 || len(restoreRequests) != len(swapRequests) {
		return convergenceMetadata{}, fmt.Errorf("loadlab: invalid repeated churn plan rounds=%d swap=%d restore=%d", rounds, len(swapRequests), len(restoreRequests))
	}

	reports := make([]phasedServerReport, 0, rounds)
	var latest convergenceMetadata
	for round := 1; round <= rounds; round++ {
		requests := swapRequests
		direction := "swap"
		if round%2 == 0 {
			requests = restoreRequests
			direction = "restore"
		}

		collector.Reset()
		slowTicks.Reset()
		server.ResetNetworkMetrics()
		convergence.Start()
		started := time.Now()
		if err := worldRuntime.EnqueueTeleportBatch(requests); err != nil {
			convergence.Stop()
			return convergenceMetadata{}, fmt.Errorf("enqueue teleport churn round %d: %w", round, err)
		}
		transition := fmt.Sprintf("teleport-churn-%02d-%s", round, direction)
		log.Printf("teleport churn round %d/%d triggered: direction=%s moved=%d timeout=%s stable=%s", round, rounds, direction, len(requests), timeout.String(), stableFor.String())
		meta, err := waitForTransitionConvergence(ctx, convergence, server, clients, timeout, stableFor, started, transition)
		convergence.Stop()
		if err != nil {
			return convergenceMetadata{}, err
		}
		latest = meta

		report := withPhaseReport(withNetworkMetrics(collector.Finish(scenario, clients), server.NetworkMetrics()), fmt.Sprintf("churn-round-%02d", round), meta)
		slowReport := slowTicks.Finish()
		roundPath := churnRoundReportPath(reportPath, round)
		if err := loadlab.WriteReport(roundPath, report); err != nil {
			return convergenceMetadata{}, fmt.Errorf("write churn round %d report: %w", round, err)
		}
		if err := writeSlowTickReport(slowTickReportPath(roundPath), slowReport); err != nil {
			return convergenceMetadata{}, fmt.Errorf("write churn round %d slow tick report: %w", round, err)
		}
		if round == 1 {
			// S3-E.7 tooling 固定讀 *-churn.json 並要求 phase=churn / transition=teleport-churn。
			// Repeated churn 的 round artifact 保留更精確名稱，但 legacy alias 不改既有 contract。
			legacyReport := report
			legacyReport.Phase = "churn"
			legacyReport.Convergence.Transition = "teleport-churn"
			legacyPath := churnReportPath(reportPath)
			if err := loadlab.WriteReport(legacyPath, legacyReport); err != nil {
				return convergenceMetadata{}, fmt.Errorf("write legacy churn report: %w", err)
			}
			if err := writeSlowTickReport(slowTickReportPath(legacyPath), slowReport); err != nil {
				return convergenceMetadata{}, fmt.Errorf("write legacy churn slow tick report: %w", err)
			}
		}
		reports = append(reports, report)
		log.Printf("teleport churn round %d/%d converged: %.3fs p99=%.3fms heap_alloc=%d heap_sys=%d gc=%d spawn=%d despawn=%d vitals=%d", round, rounds, meta.TriggerToConvergedSeconds, report.TickDuration.P99MS, report.Memory.HeapAllocBytes, report.Memory.HeapSysBytes, report.Memory.NumGC, report.Lifecycle.SpawnSelected, report.Lifecycle.DespawnSelected, report.Lifecycle.InitialVitalsSelected)
	}

	summary := buildChurnSoakSummary(scenario, clients, reports)
	if err := loadlab.WriteReport(churnSoakReportPath(reportPath), summary); err != nil {
		return convergenceMetadata{}, fmt.Errorf("write churn soak summary: %w", err)
	}
	return latest, nil
}

func buildChurnSoakSummary(scenario loadlab.Scenario, clients int, reports []phasedServerReport) churnSoakSummary {
	summary := churnSoakSummary{
		SchemaVersion:   loadlab.ReportSchemaVersion,
		Scenario:        scenario,
		ExpectedClients: clients,
		Rounds:          len(reports),
		Round:           make([]churnSoakRoundSummary, 0, len(reports)),
	}
	for i, report := range reports {
		world := report.Convergence.Observation.World
		round := churnSoakRoundSummary{
			Round:                       i + 1,
			Direction:                   "swap",
			TriggerToConvergedSeconds:   report.Convergence.TriggerToConvergedSeconds,
			ObservedNonConverged:        report.Convergence.ObservedNonConverged,
			DesiredRelationships:        world.DesiredRelationships,
			KnownDesired:                world.KnownDesired,
			PendingSpawns:               world.PendingSpawns,
			PendingDespawns:             world.PendingDespawns,
			PendingVitalsEntities:       world.PendingVitalsEntities,
			PendingDynamicSessions:      world.PendingDynamicSessions,
			ReliableQueued:              report.Convergence.Reliable.Queued,
			ReliableInFlight:            report.Convergence.Reliable.InFlight,
			TickP99MS:                   report.TickDuration.P99MS,
			TickMaxMS:                   report.TickDuration.MaxMS,
			SpawnSelected:               report.Lifecycle.SpawnSelected,
			DespawnSelected:             report.Lifecycle.DespawnSelected,
			InitialVitalsSelected:       report.Lifecycle.InitialVitalsSelected,
			LifecycleBackpressureStops:  report.Lifecycle.BackpressureStops,
			MaxLifecycleSelectedPerTick: report.Lifecycle.MaxGlobalSelectedPerTick,
			MaxInitialVitalsPerTick:     report.Lifecycle.MaxInitialVitalsSelectedPerTick,
			TotalAllocBytes:             report.Memory.TotalAllocBytes,
			Mallocs:                     report.Memory.Mallocs,
			HeapAllocBytes:              report.Memory.HeapAllocBytes,
			HeapSysBytes:                report.Memory.HeapSysBytes,
			NumGC:                       report.Memory.NumGC,
			GCPauseTotalMS:              report.Memory.GCPauseTotalMS,
			CommandErrors:               report.Errors.CommandErrors,
			UnexpectedTickErrors:        report.Errors.UnexpectedTickErrors,
			DeliveryErrors:              report.Errors.DeliveryErrors,
			NetworkErrors:               report.Errors.NetworkErrors,
			DatagramTooLarge:            report.Errors.DatagramTooLarge,
		}
		if (i+1)%2 == 0 {
			round.Direction = "restore"
		}
		summary.Round = append(summary.Round, round)
		if round.TriggerToConvergedSeconds > summary.MaxTriggerToConvergedSeconds {
			summary.MaxTriggerToConvergedSeconds = round.TriggerToConvergedSeconds
		}
		if round.TickP99MS > summary.MaxTickP99MS {
			summary.MaxTickP99MS = round.TickP99MS
		}
		if round.TickMaxMS > summary.MaxTickMS {
			summary.MaxTickMS = round.TickMaxMS
		}
		summary.TotalSpawnSelected += round.SpawnSelected
		summary.TotalDespawnSelected += round.DespawnSelected
		summary.TotalInitialVitalsSelected += round.InitialVitalsSelected
		summary.TotalAllocBytes += round.TotalAllocBytes
		if round.TotalAllocBytes > summary.MaxRoundTotalAllocBytes {
			summary.MaxRoundTotalAllocBytes = round.TotalAllocBytes
		}
		if round.HeapAllocBytes > summary.MaxHeapAllocBytes {
			summary.MaxHeapAllocBytes = round.HeapAllocBytes
		}
		if round.HeapSysBytes > summary.MaxHeapSysBytes {
			summary.MaxHeapSysBytes = round.HeapSysBytes
		}
		summary.TotalGC += uint64(round.NumGC)
		summary.TotalGCPauseMS += round.GCPauseTotalMS
		summary.TotalLifecycleBackpressure += round.LifecycleBackpressureStops
		if round.MaxLifecycleSelectedPerTick > summary.MaxLifecycleSelectedPerTick {
			summary.MaxLifecycleSelectedPerTick = round.MaxLifecycleSelectedPerTick
		}
		if round.MaxInitialVitalsPerTick > summary.MaxInitialVitalsSelectedTick {
			summary.MaxInitialVitalsSelectedTick = round.MaxInitialVitalsPerTick
		}
	}
	if len(summary.Round) > 0 {
		first := summary.Round[0]
		last := summary.Round[len(summary.Round)-1]
		summary.FirstRoundHeapAllocBytes = first.HeapAllocBytes
		summary.LastRoundHeapAllocBytes = last.HeapAllocBytes
		summary.HeapAllocGrowthBytes = int64(last.HeapAllocBytes) - int64(first.HeapAllocBytes)
		summary.FirstRoundHeapSysBytes = first.HeapSysBytes
		summary.LastRoundHeapSysBytes = last.HeapSysBytes
		summary.HeapSysGrowthBytes = int64(last.HeapSysBytes) - int64(first.HeapSysBytes)
	}
	return summary
}

func churnRoundReportPath(reportPath string, round int) string {
	return phaseReportPath(reportPath, fmt.Sprintf("churn-round-%02d", round))
}

func churnSoakReportPath(reportPath string) string {
	return phaseReportPath(reportPath, "churn-soak")
}
