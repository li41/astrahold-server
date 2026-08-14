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
	Transition                string                 `json:"transition,omitempty"`
	ReadyToConvergedSeconds   float64                `json:"ready_to_converged_seconds,omitempty"`
	TriggerToConvergedSeconds float64                `json:"trigger_to_converged_seconds,omitempty"`
	ObservedNonConverged      bool                   `json:"observed_non_converged,omitempty"`
	StableSeconds             float64                `json:"stable_seconds"`
	Observation               convergenceObservation `json:"observation"`
	Reliable                  tcpudp.ReliableBacklog `json:"reliable"`
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
	return waitForConvergenceGate(ctx, tracker, server, expected, timeout, stableFor, started, "initial", false)
}

// waitForTransitionConvergence 用於已經處於 converged 狀態後觸發的 world transition。
// 它要求至少觀察到一次 non-converged state，才允許 stable window 開始，避免 command 尚未被 owner tick
// 套用前就把舊的 converged snapshot 誤判為 transition 已完成。
func waitForTransitionConvergence(ctx context.Context, tracker *convergenceTracker, server *tcpudp.Server, expected int, timeout, stableFor time.Duration, started time.Time, transition string) (convergenceMetadata, error) {
	if transition == "" {
		transition = "transition"
	}
	return waitForConvergenceGate(ctx, tracker, server, expected, timeout, stableFor, started, transition, true)
}

func waitForConvergenceGate(ctx context.Context, tracker *convergenceTracker, server *tcpudp.Server, expected int, timeout, stableFor time.Duration, started time.Time, transition string, requireNonConverged bool) (convergenceMetadata, error) {
	if tracker == nil || server == nil || expected <= 0 || timeout <= 0 || stableFor < 0 {
		return convergenceMetadata{}, fmt.Errorf("loadlab: invalid convergence gate configuration")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	observedNonConverged := !requireNonConverged
	var stableSince time.Time
	for {
		observation, ok := tracker.Snapshot()
		backlog := server.ReliableBacklog()
		ready := ok && observation.World.Converged(expected) && backlog.Drained(expected)
		if !ready && ok {
			observedNonConverged = true
		}
		if ready && observedNonConverged {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= stableFor {
				elapsed := time.Since(started).Seconds()
				metadata := convergenceMetadata{
					Transition:           transition,
					ObservedNonConverged: requireNonConverged && observedNonConverged,
					StableSeconds:        time.Since(stableSince).Seconds(),
					Observation:          observation,
					Reliable:             backlog,
				}
				if requireNonConverged {
					metadata.TriggerToConvergedSeconds = elapsed
				} else {
					metadata.ReadyToConvergedSeconds = elapsed
				}
				return metadata, nil
			}
		} else {
			stableSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return convergenceMetadata{}, ctx.Err()
		case <-deadline.C:
			observation, _ := tracker.Snapshot()
			return convergenceMetadata{}, fmt.Errorf("loadlab: convergence timeout transition=%s observed_non_converged=%v world=%+v reliable=%+v", transition, observedNonConverged, observation.World, server.ReliableBacklog())
		case <-ticker.C:
		}
	}
}

func convergenceReportPath(reportPath string) string {
	return phaseReportPath(reportPath, "convergence")
}

func churnReportPath(reportPath string) string {
	return phaseReportPath(reportPath, "churn")
}

func phaseReportPath(reportPath, phase string) string {
	ext := filepath.Ext(reportPath)
	base := strings.TrimSuffix(reportPath, ext)
	if base == "" {
		base = reportPath
	}
	return base + "-" + phase + ext
}
