package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func main() {
	var (
		tcpAddress    = flag.String("tcp", "127.0.0.1:17777", "Reliable TCP listen address")
		udpAddress    = flag.String("udp", "127.0.0.1:17778", "Realtime UDP listen address")
		tickRate      = flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
		snapshotRate  = flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
		worldPath     = flag.String("world", "worlds/castle-sandbox/gameplay.json", "Gameplay World JSON path")
		combatPath    = flag.String("combat-actions", "config/combat-actions.json", "Combat Action Catalog JSON path")
		clients       = flag.Int("clients", 500, "Expected ready clients before measurement starts")
		scenarioText  = flag.String("scenario", string(loadlab.ScenarioGateZerg), "distributed | gate-zerg | vertical-siege")
		duration      = flag.Duration("duration", 60*time.Second, "Measurement window after all clients are ready")
		readyTimeout  = flag.Duration("ready-timeout", 45*time.Second, "Maximum time to wait for all clients")
		shutdownGrace = flag.Duration("shutdown-grace", 2*time.Second, "Keep listeners alive after report so bots can close cleanly")
		reportPath    = flag.String("report", "artifacts/loadlab-server.json", "Server JSON report path")
	)
	flag.Parse()

	if err := validateRates(*tickRate, *snapshotRate); err != nil { log.Fatal(err) }
	if *clients <= 0 || *duration <= 0 || *readyTimeout <= 0 || *shutdownGrace < 0 { log.Fatal("clients, duration and ready-timeout must be > 0; shutdown-grace must be >= 0") }
	scenario, err := loadlab.ParseScenario(*scenarioText); if err != nil { log.Fatal(err) }

	loadedWorld, err := gameplayworld.LoadFile(*worldPath); if err != nil { log.Fatalf("load gameplay world %q: %v", *worldPath, err) }
	loadedCombat, err := combat.LoadFile(*combatPath); if err != nil { log.Fatalf("load combat actions %q: %v", *combatPath, err) }
	combatService, err := combat.NewService(loadedCombat.Definition.Actions); if err != nil { log.Fatalf("build combat service: %v", err) }
	nav, err := navigation.NewGameplayNavigator(loadedWorld.Definition); if err != nil { log.Fatalf("build gameplay navigator: %v", err) }
	playerFactory, err := loadlab.NewPlayerFactory(loadedWorld.Definition, scenario, *clients); if err != nil { log.Fatal(err) }

	move := movement.NewService(nav, 0.1)
	sim := simulation.New(spatial.NewGrid(32), move)
	runtimeConfig := worldruntime.DefaultConfig(); runtimeConfig.SnapshotEveryTicks = uint64(*tickRate / *snapshotRate); runtimeConfig.CollectMetrics = true
	worldRuntime := worldruntime.New(sim, runtimeConfig, worldruntime.WithDynamicWorld(nav), worldruntime.WithSiegeGates(loadedWorld.Definition.Gates), worldruntime.WithCombatService(combatService))
	loop, err := worldruntime.NewLoop(worldRuntime, *tickRate); if err != nil { log.Fatal(err) }

	networkConfig := tcpudp.DefaultConfig(); networkConfig.TCPAddress=*tcpAddress; networkConfig.UDPAddress=*udpAddress; networkConfig.TickRateHz=uint16(*tickRate); networkConfig.SnapshotRateHz=uint16(*snapshotRate); networkConfig.PlayerFactory=playerFactory
	networkConfig.WorldIdentity = protocol.WorldIdentity{WorldID: loadedWorld.Definition.WorldID, Revision: loadedWorld.Definition.Revision, GameplaySHA256: loadedWorld.SHA256}
	server := tcpudp.NewServer(networkConfig, worldRuntime, gamev1.Codec{}); if err := server.Open(); err != nil { log.Fatal(err) }; defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM); defer stop()
	collector := loadlab.NewServerCollector(*tickRate, *snapshotRate)
	loopDone := make(chan error,1); go func(){ err:=loop.RunObserved(ctx,collector.RecordStep); if err!=nil{stop()}; loopDone<-err }()
	serveDone := make(chan error,1); go func(){ err:=server.Serve(ctx); if err!=nil{stop()}; serveDone<-err }()
	go collectNetworkErrors(ctx,server.Errors(),collector)

	log.Printf("Siege Load Server ready: protocol=%d codec=gamev1 combat_revision=%s scenario=%s clients=%d tcp=%s udp=%s tick=%dHz snapshot=%dHz gates=%d", protocol.Version, loadedCombat.Definition.Revision, scenario, *clients, server.TCPAddr(), server.UDPAddr(), *tickRate, *snapshotRate, len(loadedWorld.Definition.Gates))
	if err := waitForClients(ctx,server,*clients,*readyTimeout); err != nil { stop(); <-serveDone; <-loopDone; log.Fatal(err) }
	log.Printf("all clients ready; starting %s measurement window", duration.String()); collector.Reset()

	measurementTimer:=time.NewTimer(*duration); completed:=false
	select { case <-measurementTimer.C: completed=true; case <-ctx.Done(): measurementTimer.Stop() }
	report:=collector.Finish(scenario,*clients)
	if err:=loadlab.WriteReport(*reportPath,report); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write report: %v",err) }
	log.Printf("load report written: %s ticks=%d p99=%.3fms max_queue=%d datagram_too_large=%d",*reportPath,report.Ticks,report.TickDuration.P99MS,report.Queue.MaxDepthBefore,report.Errors.DatagramTooLarge)

	if completed && *shutdownGrace>0 { timer:=time.NewTimer(*shutdownGrace); select { case <-timer.C: case <-ctx.Done(): timer.Stop() } }
	stop(); serveErr:=<-serveDone; loopErr:=<-loopDone
	if serveErr!=nil { log.Printf("network server stopped with error: %v",serveErr) }
	if loopErr!=nil { log.Printf("world loop stopped with error: %v",loopErr) }
	if !completed { os.Exit(2) }
}

func validateRates(tickRate,snapshotRate int) error {
	if tickRate<=0 || snapshotRate<=0 { return fmt.Errorf("tick-rate and snapshot-rate must be > 0") }
	if tickRate>65535 || snapshotRate>65535 { return fmt.Errorf("tick-rate and snapshot-rate must be <= 65535") }
	if snapshotRate>tickRate || tickRate%snapshotRate!=0 { return fmt.Errorf("snapshot-rate must divide tick-rate evenly and be <= tick-rate") }
	return nil
}
func waitForClients(ctx context.Context,server *tcpudp.Server,expected int,timeout time.Duration) error {
	deadline:=time.NewTimer(timeout); defer deadline.Stop(); ticker:=time.NewTicker(25*time.Millisecond); defer ticker.Stop()
	for { if ready:=server.ReadyPeerCount(); ready>=expected { return nil }; select { case <-ctx.Done(): return ctx.Err(); case <-deadline.C: return fmt.Errorf("loadlab: ready timeout: got=%d want=%d",server.ReadyPeerCount(),expected); case <-ticker.C: } }
}
func collectNetworkErrors(ctx context.Context,events <-chan tcpudp.NetworkError,collector *loadlab.ServerCollector) {
	for { select { case <-ctx.Done(): return; case event:=<-events: collector.RecordNetworkError(event.Operation,event.Err) } }
}
