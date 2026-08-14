package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type convergenceObservation struct {
	Tick  uint64                           `json:"tick"`
	World worldruntime.ConvergenceSnapshot `json:"world"`
}

type convergenceMetadata struct {
	ReadyToConvergedSeconds float64                `json:"ready_to_converged_seconds"`
	StableSeconds           float64                `json:"stable_seconds"`
	Observation             convergenceObservation `json:"observation"`
	Reliable                tcpudp.ReliableBacklog `json:"reliable"`
}

type convergenceTracker struct {
	enabled atomic.Bool
	mu      sync.RWMutex
	latest  convergenceObservation
	hasData bool
}

func newConvergenceTracker() *convergenceTracker { return &convergenceTracker{} }

func (t *convergenceTracker) Start() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.latest = convergenceObservation{}
	t.hasData = false
	t.mu.Unlock()
	t.enabled.Store(true)
}

func (t *convergenceTracker) Stop() {
	if t != nil {
		t.enabled.Store(false)
	}
}

// Record 只能從 Loop.RunObserved callback 呼叫。callback 與 Runtime.Step 同 goroutine，
// 因此可以安全讀取 Runtime owner-only convergence state；main goroutine 只讀 tracker copy。
func (t *convergenceTracker) Record(tick uint64, runtime *worldruntime.Runtime) {
	if t == nil || runtime == nil || !t.enabled.Load() {
		return
	}
	observation := convergenceObservation{Tick: tick, World: runtime.ConvergenceSnapshot()}
	t.mu.Lock()
	t.latest = observation
	t.hasData = true
	t.mu.Unlock()
}

func (t *convergenceTracker) Snapshot() (convergenceObservation, bool) {
	if t == nil {
		return convergenceObservation{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.latest, t.hasData
}

func waitForConvergence(ctx context.Context, tracker *convergenceTracker, server *tcpudp.Server, expected int, timeout, stableFor time.Duration, started time.Time) (convergenceMetadata, error) {
	if tracker == nil || server == nil || expected <= 0 || timeout <= 0 || stableFor < 0 {
		return convergenceMetadata{}, fmt.Errorf("loadlab: invalid convergence gate configuration")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	var stableSince time.Time
	for {
		observation, ok := tracker.Snapshot()
		backlog := server.ReliableBacklog()
		ready := ok && observation.World.Converged(expected) && backlog.Drained(expected)
		if ready {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stableFor {
				return convergenceMetadata{
					ReadyToConvergedSeconds: time.Since(started).Seconds(),
					StableSeconds:           time.Since(stableSince).Seconds(),
					Observation:             observation,
					Reliable:                backlog,
				}, nil
			}
		} else {
			stableSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return convergenceMetadata{}, ctx.Err()
		case <-deadline.C:
			observation, _ := tracker.Snapshot()
			return convergenceMetadata{}, fmt.Errorf("loadlab: convergence timeout: world=%+v reliable=%+v", observation.World, server.ReliableBacklog())
		case <-ticker.C:
		}
	}
}

func convergenceReportPath(reportPath string) string {
	ext := filepath.Ext(reportPath)
	base := strings.TrimSuffix(reportPath, ext)
	if base == "" {
		base = reportPath
	}
	return base + "-convergence" + ext
}
