package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func main() {
	var (
		tcpAddress   = flag.String("tcp", "127.0.0.1:7777", "Reliable TCP listen address")
		udpAddress   = flag.String("udp", "127.0.0.1:7778", "Realtime UDP listen address")
		tickRate     = flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
		snapshotRate = flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
	)
	flag.Parse()

	if err := validateRates(*tickRate, *snapshotRate); err != nil {
		log.Fatal(err)
	}

	nav := navigation.Plane{
		MinX: -512, MaxX: 512,
		MinZ: -512, MaxZ: 512,
		Height: 0,
		Layer:  0,
	}
	move := movement.NewService(nav, 0.1)
	sim := simulation.New(spatial.NewGrid(32), move)

	runtimeConfig := worldruntime.DefaultConfig()
	runtimeConfig.SnapshotEveryTicks = uint64(*tickRate / *snapshotRate)
	runtime := worldruntime.New(sim, runtimeConfig)
	loop, err := worldruntime.NewLoop(runtime, *tickRate)
	if err != nil {
		log.Fatal(err)
	}

	networkConfig := tcpudp.DefaultConfig()
	networkConfig.TCPAddress = *tcpAddress
	networkConfig.UDPAddress = *udpAddress
	networkConfig.TickRateHz = uint16(*tickRate)
	networkConfig.SnapshotRateHz = uint16(*snapshotRate)
	server := tcpudp.NewServer(networkConfig, runtime, jsonv1.Codec{})
	if err := server.Open(); err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.RunObserved(ctx, logStepReport)
	}()
	go logNetworkErrors(ctx, server.Errors())

	log.Printf(
		"Astrahold worldd S2-B ready: tcp=%s udp=%s tick_rate=%dHz snapshot_rate=%dHz codec=jsonv1",
		server.TCPAddr(), server.UDPAddr(), *tickRate, *snapshotRate,
	)
	log.Printf("S2-B transport is for local/controlled development; do not expose it directly to the Internet")

	if err := server.Serve(ctx); err != nil {
		stop()
		log.Printf("network server stopped with error: %v", err)
	}
	if err := <-loopDone; err != nil {
		log.Printf("world loop stopped with error: %v", err)
	}
}

func validateRates(tickRate, snapshotRate int) error {
	if tickRate <= 0 || snapshotRate <= 0 {
		return fmt.Errorf("tick-rate and snapshot-rate must be > 0")
	}
	if tickRate > 65535 || snapshotRate > 65535 {
		return fmt.Errorf("tick-rate and snapshot-rate must be <= 65535")
	}
	if snapshotRate > tickRate || tickRate%snapshotRate != 0 {
		return fmt.Errorf("snapshot-rate must divide tick-rate evenly and be <= tick-rate")
	}
	return nil
}

func logStepReport(report worldruntime.StepReport) {
	for _, item := range report.CommandErrors {
		log.Printf("world command error: tick=%d command=%s session=%d err=%v", report.Tick, item.Command, item.SessionID, item.Err)
	}
	for _, item := range report.TickErrors {
		log.Printf("world tick error: tick=%d entity=%d err=%v", report.Tick, item.EntityID, item.Err)
	}
	for _, item := range report.DeliveryErrors {
		log.Printf("world delivery error: tick=%d session=%d delivery=%d type=%d err=%v", report.Tick, item.SessionID, item.Delivery, item.MessageType, item.Err)
	}
}

func logNetworkErrors(ctx context.Context, events <-chan tcpudp.NetworkError) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			log.Printf("network error: session=%d op=%s err=%v", event.SessionID, event.Operation, event.Err)
		}
	}
}
