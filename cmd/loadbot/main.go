package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/li41/astrahold-server/internal/loadlab"
)

func main() {
	var (
		tcpAddress   = flag.String("tcp", "127.0.0.1:17777", "Load Server reliable TCP address")
		clients      = flag.Int("clients", 500, "Number of headless clients")
		scenarioText = flag.String("scenario", string(loadlab.ScenarioGateZerg), "distributed | gate-zerg | vertical-siege")
		inputRate    = flag.Int("input-rate", 20, "Movement input rate per client (Hz)")
		rampUp       = flag.Duration("ramp-up", 5*time.Second, "Connection ramp-up window")
		duration     = flag.Duration("duration", 90*time.Second, "Maximum bot process duration; 0 waits until server closes")
		connectTimeout = flag.Duration("connect-timeout", 5*time.Second, "Per-client TCP connect timeout")
		reportPath   = flag.String("report", "artifacts/loadlab-bots.json", "Bot JSON report path")
	)
	flag.Parse()

	if *clients <= 0 || *inputRate <= 0 || *rampUp < 0 || *duration < 0 {
		log.Fatal("clients/input-rate must be > 0; ramp-up/duration must be >= 0")
	}
	scenario, err := loadlab.ParseScenario(*scenarioText)
	if err != nil {
		log.Fatal(err)
	}

	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx := root
	var cancel context.CancelFunc
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(root, *duration)
		defer cancel()
	}

	report, err := loadlab.RunBots(ctx, loadlab.BotConfig{
		TCPAddress:     *tcpAddress,
		Clients:        *clients,
		Scenario:       scenario,
		InputRateHz:    *inputRate,
		RampUp:         *rampUp,
		ConnectTimeout: *connectTimeout,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := loadlab.WriteReport(*reportPath, report); err != nil {
		log.Fatal(err)
	}
	log.Printf("bot report written: %s ready=%d/%d moves=%d snapshots=%d corrections=%d udp_rx=%d bytes", *reportPath, report.ReadyClients, report.RequestedClients, report.MovesSent, report.Snapshots, report.Corrections, report.UDPBytesReceived)
	if report.ReadyClients != uint64(*clients) || report.FailedConnections != 0 {
		os.Exit(2)
	}
}
