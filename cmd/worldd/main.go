package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const worldTickRateHz = 20

func main() {
	nav := navigation.Plane{MinX: -512, MaxX: 512, MinZ: -512, MaxZ: 512, Height: 0, Layer: 0}
	move := movement.NewService(nav, 0.1)
	sim := simulation.New(spatial.NewGrid(32), move)
	cfg := worldruntime.DefaultConfig()
	runtime := worldruntime.New(sim, cfg)
	loop, err := worldruntime.NewLoop(runtime, worldTickRateHz)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("Astrahold worldd ready: tick_rate=%dHz snapshot_every=%d_ticks", worldTickRateHz, cfg.SnapshotEveryTicks)
	if err := loop.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
