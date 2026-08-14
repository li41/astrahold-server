package main

import (
	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
)

type networkLoadSummary struct {
	RealtimeDatagrams   uint64  `json:"realtime_datagrams"`
	RealtimeBytes       uint64  `json:"realtime_bytes"`
	RealtimeBytesPerSec float64 `json:"realtime_bytes_per_second"`
	RealtimeMbitsPerSec float64 `json:"realtime_mbits_per_second"`
	SnapshotDatagrams   uint64  `json:"snapshot_datagrams"`
	SnapshotBytes       uint64  `json:"snapshot_bytes"`
	SnapshotMbitsPerSec float64 `json:"snapshot_mbits_per_second"`
	CorrectionDatagrams uint64  `json:"correction_datagrams"`
	CorrectionBytes     uint64  `json:"correction_bytes"`
	CorrectionMbitsPerSec float64 `json:"correction_mbits_per_second"`
	EncodeTotalMS       float64 `json:"encode_total_ms"`
	EncodeAverageUS     float64 `json:"encode_average_us"`
}

type measuredServerReport struct {
	loadlab.ServerReport
	Network networkLoadSummary `json:"network"`
}

func withNetworkMetrics(report loadlab.ServerReport, metrics tcpudp.NetworkMetrics) measuredServerReport {
	seconds := report.MeasurementSeconds
	if seconds <= 0 {
		seconds = 1
	}
	encodeAverageUS := float64(0)
	if metrics.RealtimeDatagrams > 0 {
		encodeAverageUS = float64(metrics.EncodeNanoseconds) / 1000 / float64(metrics.RealtimeDatagrams)
	}
	return measuredServerReport{
		ServerReport: report,
		Network: networkLoadSummary{
			RealtimeDatagrams:     metrics.RealtimeDatagrams,
			RealtimeBytes:         metrics.RealtimeBytes,
			RealtimeBytesPerSec:   float64(metrics.RealtimeBytes) / seconds,
			RealtimeMbitsPerSec:   float64(metrics.RealtimeBytes) * 8 / seconds / 1e6,
			SnapshotDatagrams:     metrics.SnapshotDatagrams,
			SnapshotBytes:         metrics.SnapshotBytes,
			SnapshotMbitsPerSec:   float64(metrics.SnapshotBytes) * 8 / seconds / 1e6,
			CorrectionDatagrams:   metrics.CorrectionDatagrams,
			CorrectionBytes:       metrics.CorrectionBytes,
			CorrectionMbitsPerSec: float64(metrics.CorrectionBytes) * 8 / seconds / 1e6,
			EncodeTotalMS:         float64(metrics.EncodeNanoseconds) / 1e6,
			EncodeAverageUS:       encodeAverageUS,
		},
	}
}
