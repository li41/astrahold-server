package loadlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRecordUDPFailureSuppressesCorrelatedShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &botCollector{}
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	recordUDPFailureUnlessStopping(ctx, collector, 7, "udp_read", errors.New("connection refused"))
	if got := collector.networkErrors.Load(); got != 0 {
		t.Fatalf("network errors=%d want=0 for correlated shutdown", got)
	}
	if got := len(collector.networkErrorSamplesSnapshot()); got != 0 {
		t.Fatalf("network error samples=%d want=0 for correlated shutdown", got)
	}
}

func TestRecordUDPFailureSuppressesDelayedCorrelatedShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &botCollector{}
	go func() {
		time.Sleep(75 * time.Millisecond)
		cancel()
	}()
	recordUDPFailureUnlessStoppingWithin(ctx, collector, 8, "udp_send", errors.New("connection refused"), 100*time.Millisecond)
	if got := collector.networkErrors.Load(); got != 0 {
		t.Fatalf("network errors=%d want=0 for delayed correlated shutdown", got)
	}
	if got := len(collector.networkErrorSamplesSnapshot()); got != 0 {
		t.Fatalf("network error samples=%d want=0 for delayed correlated shutdown", got)
	}
}

func TestRecordUDPFailureCountsWhileTCPContextStaysAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &botCollector{}
	err := errors.New("connection refused")
	recordUDPFailureUnlessStoppingWithin(ctx, collector, 9, "udp_read", err, 10*time.Millisecond)
	if got := collector.networkErrors.Load(); got != 1 {
		t.Fatalf("network errors=%d want=1 for standalone UDP failure", got)
	}
	samples := collector.networkErrorSamplesSnapshot()
	if len(samples) != 1 {
		t.Fatalf("network error samples=%d want=1", len(samples))
	}
	if got := samples[0]; got.BotIndex != 9 || got.Stage != "udp_read" || got.Error != err.Error() {
		t.Fatalf("sample=%+v want bot=9 stage=udp_read error=%q", got, err)
	}
}

func TestNetworkErrorSamplesAreBoundedWithoutChangingCount(t *testing.T) {
	collector := &botCollector{}
	for i := 0; i < maxNetworkErrorSamples+3; i++ {
		collector.recordNetworkError(i, "tcp_read", errors.New("read failed"))
	}
	if got, want := collector.networkErrors.Load(), uint64(maxNetworkErrorSamples+3); got != want {
		t.Fatalf("network errors=%d want=%d", got, want)
	}
	samples := collector.networkErrorSamplesSnapshot()
	if got := len(samples); got != maxNetworkErrorSamples {
		t.Fatalf("network error samples=%d want=%d", got, maxNetworkErrorSamples)
	}
	if samples[0].BotIndex != 0 || samples[len(samples)-1].BotIndex != maxNetworkErrorSamples-1 {
		t.Fatalf("bounded samples=%+v", samples)
	}
}

func TestBotReportOmitsEmptyNetworkErrorSamples(t *testing.T) {
	encoded, err := json.Marshal(BotReport{SchemaVersion: ReportSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"network_error_samples"`)) {
		t.Fatalf("empty diagnostic field must stay omitted from successful report: %s", encoded)
	}
}
