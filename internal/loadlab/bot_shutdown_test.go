package loadlab

import (
	"context"
	"testing"
	"time"
)

func TestRecordMoveSendFailureSuppressesCorrelatedShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &botCollector{}
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	recordMoveSendFailure(ctx, collector)
	if got := collector.networkErrors.Load(); got != 0 {
		t.Fatalf("network errors=%d want=0 for correlated shutdown", got)
	}
}

func TestRecordMoveSendFailureCountsWhileTCPContextStaysAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &botCollector{}
	recordMoveSendFailure(ctx, collector)
	if got := collector.networkErrors.Load(); got != 1 {
		t.Fatalf("network errors=%d want=1 for standalone UDP failure", got)
	}
}
