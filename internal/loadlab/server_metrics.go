package loadlab

import (
	"encoding/json"
	"errors"
	goruntime "runtime"
	"sort"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const ReportSchemaVersion = 1

type DurationSummary struct {
	AverageMS float64 `json:"average_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
	MaxMS     float64 `json:"max_ms"`
}

type StageSummary struct {
	SimulationAverageMS         float64 `json:"simulation_average_ms"`
	DynamicReplicationAverageMS float64 `json:"dynamic_replication_average_ms"`
	AOIAverageMS                float64 `json:"aoi_average_ms"`
	ReplicationBuildAverageMS   float64 `json:"replication_build_average_ms"`
	DeliveryAverageMS           float64 `json:"delivery_average_ms"`
}

type QueueSummary struct {
	MaxDepthBefore        int     `json:"max_depth_before"`
	MaxDepthAfter         int     `json:"max_depth_after"`
	CommandsTotal         uint64  `json:"commands_total"`
	CommandsAveragePerTick float64 `json:"commands_average_per_tick"`
}

type AOISummary struct {
	Queries             uint64  `json:"queries"`
	Candidates          uint64  `json:"candidates"`
	Visible             uint64  `json:"visible"`
	CandidatesPerQuery  float64 `json:"candidates_per_query"`
	VisiblePerQuery     float64 `json:"visible_per_query"`
	CandidateToVisible  float64 `json:"candidate_to_visible_ratio"`
}

type ReplicationSummary struct {
	SessionsReplicated uint64  `json:"sessions_replicated"`
	OutboundMessages   uint64  `json:"outbound_messages"`
	MessagesPerSecond  float64 `json:"messages_per_second"`
}

type ErrorSummary struct {
	CommandErrors       uint64            `json:"command_errors"`
	BlockedMoves        uint64            `json:"blocked_moves"`
	UnexpectedTickErrors uint64           `json:"unexpected_tick_errors"`
	DeliveryErrors      uint64            `json:"delivery_errors"`
	NetworkErrors       uint64            `json:"network_errors"`
	DatagramTooLarge    uint64            `json:"datagram_too_large"`
	NetworkByOperation  map[string]uint64 `json:"network_by_operation,omitempty"`
}

type MemorySummary struct {
	TotalAllocBytes uint64  `json:"total_alloc_bytes"`
	Mallocs         uint64  `json:"mallocs"`
	HeapAllocBytes  uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes    uint64  `json:"heap_sys_bytes"`
	NumGC           uint32  `json:"num_gc"`
	GCPauseTotalMS  float64 `json:"gc_pause_total_ms"`
	Goroutines      int     `json:"goroutines"`
}

type ServerReport struct {
	SchemaVersion     int                `json:"schema_version"`
	Scenario          Scenario           `json:"scenario"`
	ExpectedClients   int                `json:"expected_clients"`
	TickRateHz        int                `json:"tick_rate_hz"`
	SnapshotRateHz    int                `json:"snapshot_rate_hz"`
	MeasurementSeconds float64           `json:"measurement_seconds"`
	Ticks             uint64             `json:"ticks"`
	TickDuration      DurationSummary    `json:"tick_duration"`
	Stages            StageSummary       `json:"stages"`
	Queue             QueueSummary       `json:"queue"`
	AOI               AOISummary         `json:"aoi"`
	Replication       ReplicationSummary `json:"replication"`
	Errors            ErrorSummary       `json:"errors"`
	Memory            MemorySummary      `json:"memory"`
}

type ServerCollector struct {
	mu sync.Mutex

	active       bool
	started      time.Time
	startMem     goruntime.MemStats
	tickRateHz   int
	snapshotRate int

	tickDurations []time.Duration
	ticks         uint64

	simulationDuration         time.Duration
	dynamicReplicationDuration time.Duration
	aoiDuration                time.Duration
	replicationBuildDuration   time.Duration
	deliveryDuration           time.Duration

	maxQueueBefore int
	maxQueueAfter  int
	commands       uint64
	aoiQueries     uint64
	aoiCandidates  uint64
	aoiVisible     uint64
	sessions       uint64
	messages       uint64

	commandErrors        uint64
	blockedMoves         uint64
	unexpectedTickErrors uint64
	deliveryErrors       uint64
	networkErrors        uint64
	datagramTooLarge     uint64
	networkByOperation   map[string]uint64
}

func NewServerCollector(tickRateHz, snapshotRateHz int) *ServerCollector {
	return &ServerCollector{tickRateHz: tickRateHz, snapshotRate: snapshotRateHz}
}

// Reset 在所有預期 Client ready 後開始新的量測 window。
func (c *ServerCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c = ServerCollector{
		tickRateHz:         c.tickRateHz,
		snapshotRate:       c.snapshotRate,
		active:             true,
		started:            time.Now(),
		tickDurations:      make([]time.Duration, 0, c.tickRateHz*120),
		networkByOperation: make(map[string]uint64),
	}
	goruntime.ReadMemStats(&c.startMem)
}

func (c *ServerCollector) RecordStep(report worldruntime.StepReport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return
	}
	m := report.Metrics
	c.ticks++
	c.tickDurations = append(c.tickDurations, m.TotalDuration)
	c.simulationDuration += m.SimulationDuration
	c.dynamicReplicationDuration += m.DynamicReplicationDuration
	c.aoiDuration += m.AOIDuration
	c.replicationBuildDuration += m.ReplicationBuildDuration
	c.deliveryDuration += m.DeliveryDuration
	if m.CommandQueueDepthBefore > c.maxQueueBefore {
		c.maxQueueBefore = m.CommandQueueDepthBefore
	}
	if m.CommandQueueDepthAfter > c.maxQueueAfter {
		c.maxQueueAfter = m.CommandQueueDepthAfter
	}
	c.commands += uint64(m.CommandsDrained)
	c.aoiQueries += uint64(m.AOIQueries)
	c.aoiCandidates += uint64(m.AOICandidates)
	c.aoiVisible += uint64(m.AOIVisible)
	c.sessions += uint64(m.SessionsReplicated)
	c.messages += uint64(m.OutboundMessages)
	c.commandErrors += uint64(len(report.CommandErrors))
	c.deliveryErrors += uint64(len(report.DeliveryErrors))
	for _, item := range report.TickErrors {
		if errors.Is(item.Err, navigation.ErrBlocked) {
			c.blockedMoves++
		} else {
			c.unexpectedTickErrors++
		}
	}
}

func (c *ServerCollector) RecordNetworkError(operation string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return
	}
	c.networkErrors++
	if c.networkByOperation == nil {
		c.networkByOperation = make(map[string]uint64)
	}
	c.networkByOperation[operation]++
	if errors.Is(err, tcpudp.ErrDatagramTooLarge) {
		c.datagramTooLarge++
	}
}

func (c *ServerCollector) Finish(scenario Scenario, expectedClients int) ServerReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	var endMem goruntime.MemStats
	goruntime.ReadMemStats(&endMem)
	duration := time.Since(c.started)
	if !c.active || c.started.IsZero() {
		duration = 0
	}

	ticks := c.ticks
	tickFloat := float64(ticks)
	if tickFloat == 0 {
		tickFloat = 1
	}
	seconds := duration.Seconds()
	if seconds <= 0 {
		seconds = 1
	}

	queries := float64(c.aoiQueries)
	if queries == 0 {
		queries = 1
	}
	visible := float64(c.aoiVisible)
	if visible == 0 {
		visible = 1
	}

	networkOps := make(map[string]uint64, len(c.networkByOperation))
	for key, value := range c.networkByOperation {
		networkOps[key] = value
	}

	return ServerReport{
		SchemaVersion:       ReportSchemaVersion,
		Scenario:            scenario,
		ExpectedClients:     expectedClients,
		TickRateHz:          c.tickRateHz,
		SnapshotRateHz:      c.snapshotRate,
		MeasurementSeconds:  duration.Seconds(),
		Ticks:               ticks,
		TickDuration:        summarizeDurations(c.tickDurations),
		Stages: StageSummary{
			SimulationAverageMS:         durationMS(c.simulationDuration) / tickFloat,
			DynamicReplicationAverageMS: durationMS(c.dynamicReplicationDuration) / tickFloat,
			AOIAverageMS:                durationMS(c.aoiDuration) / tickFloat,
			ReplicationBuildAverageMS:   durationMS(c.replicationBuildDuration) / tickFloat,
			DeliveryAverageMS:           durationMS(c.deliveryDuration) / tickFloat,
		},
		Queue: QueueSummary{
			MaxDepthBefore:         c.maxQueueBefore,
			MaxDepthAfter:          c.maxQueueAfter,
			CommandsTotal:          c.commands,
			CommandsAveragePerTick: float64(c.commands) / tickFloat,
		},
		AOI: AOISummary{
			Queries:            c.aoiQueries,
			Candidates:         c.aoiCandidates,
			Visible:            c.aoiVisible,
			CandidatesPerQuery: float64(c.aoiCandidates) / queries,
			VisiblePerQuery:    float64(c.aoiVisible) / queries,
			CandidateToVisible: float64(c.aoiCandidates) / visible,
		},
		Replication: ReplicationSummary{
			SessionsReplicated: c.sessions,
			OutboundMessages:   c.messages,
			MessagesPerSecond:  float64(c.messages) / seconds,
		},
		Errors: ErrorSummary{
			CommandErrors:        c.commandErrors,
			BlockedMoves:         c.blockedMoves,
			UnexpectedTickErrors: c.unexpectedTickErrors,
			DeliveryErrors:       c.deliveryErrors,
			NetworkErrors:        c.networkErrors,
			DatagramTooLarge:     c.datagramTooLarge,
			NetworkByOperation:   networkOps,
		},
		Memory: MemorySummary{
			TotalAllocBytes: delta64(endMem.TotalAlloc, c.startMem.TotalAlloc),
			Mallocs:         delta64(endMem.Mallocs, c.startMem.Mallocs),
			HeapAllocBytes:  endMem.HeapAlloc,
			HeapSysBytes:    endMem.HeapSys,
			NumGC:           delta32(endMem.NumGC, c.startMem.NumGC),
			GCPauseTotalMS:  float64(delta64(endMem.PauseTotalNs, c.startMem.PauseTotalNs)) / 1e6,
			Goroutines:      goruntime.NumGoroutine(),
		},
	}
}

func summarizeDurations(values []time.Duration) DurationSummary {
	if len(values) == 0 {
		return DurationSummary{}
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total time.Duration
	for _, value := range ordered {
		total += value
	}
	return DurationSummary{
		AverageMS: durationMS(total) / float64(len(ordered)),
		P50MS:     durationMS(percentile(ordered, 0.50)),
		P95MS:     durationMS(percentile(ordered, 0.95)),
		P99MS:     durationMS(percentile(ordered, 0.99)),
		MaxMS:     durationMS(ordered[len(ordered)-1]),
	}
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(mathCeil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func mathCeil(value float64) float64 {
	integer := float64(int64(value))
	if value > integer {
		return integer + 1
	}
	return integer
}

func durationMS(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }

func delta64(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func delta32(after, before uint32) uint32 {
	if after < before {
		return 0
	}
	return after - before
}

func MarshalReport(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}
