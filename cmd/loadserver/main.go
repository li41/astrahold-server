package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
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
		tcpAddress         = flag.String("tcp", "127.0.0.1:17777", "Reliable TCP listen address")
		udpAddress         = flag.String("udp", "127.0.0.1:17778", "Realtime UDP listen address")
		tickRate           = flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
		snapshotRate       = flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
		worldPath          = flag.String("world", "worlds/castle-sandbox/gameplay.json", "Gameplay World JSON path")
		combatPath         = flag.String("combat-actions", "config/combat-actions.json", "Combat Action Catalog JSON path")
		clients            = flag.Int("clients", 500, "Expected ready clients before convergence starts")
		scenarioText       = flag.String("scenario", string(loadlab.ScenarioGateZerg), "distributed | gate-zerg | vertical-siege | teleport-churn")
		duration           = flag.Duration("duration", 60*time.Second, "Steady-state measurement window after semantic convergence / optional churn")
		readyTimeout       = flag.Duration("ready-timeout", 45*time.Second, "Maximum time to wait for all clients")
		convergenceTimeout = flag.Duration("convergence-timeout", 30*time.Second, "Maximum time from all-ready to lifecycle/reliable convergence")
		convergenceStable  = flag.Duration("convergence-stable", 250*time.Millisecond, "How long initial convergence conditions must remain continuously true")
		churnTimeout       = flag.Duration("churn-timeout", 15*time.Second, "Maximum time from teleport-churn trigger to lifecycle/reliable convergence")
		churnStable        = flag.Duration("churn-stable", 250*time.Millisecond, "How long post-churn convergence conditions must remain continuously true")
		shutdownGrace      = flag.Duration("shutdown-grace", 2*time.Second, "Keep listeners alive after report so bots can close cleanly")
		reportPath         = flag.String("report", "artifacts/loadlab-server.json", "Steady-state Server JSON report path")
		allocProfilePrefix = flag.String("alloc-profile-prefix", "", "Optional steady-state allocation profile path prefix; writes <prefix>-before.pprof and <prefix>-after.pprof")
		allocProfileRate   = flag.Int("alloc-profile-rate", 64*1024, "Allocation profiler sampling rate in bytes")
	)
	flag.Parse()

	if err := validateRates(*tickRate, *snapshotRate); err != nil { log.Fatal(err) }
	if *clients <= 0 || *duration <= 0 || *readyTimeout <= 0 || *convergenceTimeout <= 0 || *churnTimeout <= 0 || *convergenceStable < 0 || *churnStable < 0 || *shutdownGrace < 0 {
		log.Fatal("clients/duration/ready-timeout/convergence-timeout/churn-timeout must be > 0; convergence-stable/churn-stable/shutdown-grace must be >= 0")
	}
	if *allocProfileRate <= 0 { log.Fatal("alloc-profile-rate must be > 0") }
	profiler, err := newAllocProfiler(*allocProfilePrefix, *allocProfileRate); if err != nil { log.Fatalf("configure allocation profiler: %v", err) }
	if profiler != nil { defer profiler.Close() }
	scenario, err := loadlab.ParseScenario(*scenarioText); if err != nil { log.Fatal(err) }

	loadedWorld, err := gameplayworld.LoadFile(*worldPath); if err != nil { log.Fatalf("load gameplay world %q: %v", *worldPath, err) }
	loadedCombat, err := combat.LoadFile(*combatPath); if err != nil { log.Fatalf("load combat actions %q: %v", *combatPath, err) }
	combatService, err := combat.NewService(loadedCombat.Definition.Actions); if err != nil { log.Fatalf("build combat service: %v", err) }
	nav, err := navigation.NewGameplayNavigator(loadedWorld.Definition); if err != nil { log.Fatalf("build gameplay navigator: %v", err) }
	playerFactory, err := loadlab.NewPlayerFactory(loadedWorld.Definition, scenario, *clients); if err != nil { log.Fatal(err) }

	var churnRequests []worldruntime.TeleportRequest
	if scenario == loadlab.ScenarioTeleportChurn {
		targets, targetErr := loadlab.TeleportChurnTargets(loadedWorld.Definition, *clients)
		if targetErr != nil { log.Fatal(targetErr) }
		churnRequests = make([]worldruntime.TeleportRequest, 0, len(targets))
		for entityID, position := range targets {
			churnRequests = append(churnRequests, worldruntime.TeleportRequest{EntityID: entityID, Position: position})
		}
		sort.Slice(churnRequests, func(i, j int) bool { return churnRequests[i].EntityID < churnRequests[j].EntityID })
	}

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
	slowTicks := newSlowTickCollector(defaultSlowTickLimit)
	convergence := newConvergenceTracker()
	loopDone := make(chan error,1); go func(){
		err:=loop.RunObserved(ctx,func(report worldruntime.StepReport){
			collector.RecordStep(report)
			slowTicks.Record(report)
			if report.Tick%runtimeConfig.SnapshotEveryTicks == 0 {
				convergence.Record(report.Tick, worldRuntime)
			}
		})
		if err!=nil{stop()}
		loopDone<-err
	}()
	serveDone := make(chan error,1); go func(){ err:=server.Serve(ctx); if err!=nil{stop()}; serveDone<-err }()
	go collectNetworkErrors(ctx,server.Errors(),collector)

	log.Printf("Siege Load Server ready: protocol=%d codec=gamev1 combat_revision=%s scenario=%s clients=%d tcp=%s udp=%s tick=%dHz snapshot=%dHz gates=%d", protocol.Version, loadedCombat.Definition.Revision, scenario, *clients, server.TCPAddr(), server.UDPAddr(), *tickRate, *snapshotRate, len(loadedWorld.Definition.Gates))
	if err := waitForClients(ctx,server,*clients,*readyTimeout); err != nil { stop(); <-serveDone; <-loopDone; log.Fatal(err) }

	// Phase 1：all-ready 只代表 SessionWelcome / UDP endpoint 已建立。
	// 在 lifecycle + vitals + dynamic + Reliable transport 真正 drain 前，先獨立量測 initial convergence。
	convergenceStarted := time.Now()
	collector.Reset(); slowTicks.Reset(); server.ResetNetworkMetrics(); convergence.Start()
	log.Printf("all clients ready; starting semantic convergence phase timeout=%s stable=%s", convergenceTimeout.String(), convergenceStable.String())
	convergenceMeta, err := waitForConvergence(ctx, convergence, server, *clients, *convergenceTimeout, *convergenceStable, convergenceStarted)
	convergence.Stop()
	if err != nil { stop(); <-serveDone; <-loopDone; log.Fatal(err) }
	convergenceReport := withPhaseReport(withNetworkMetrics(collector.Finish(scenario,*clients), server.NetworkMetrics()), "convergence", convergenceMeta)
	convergenceSlowReport := slowTicks.Finish()
	convergencePath := convergenceReportPath(*reportPath)
	if err:=loadlab.WriteReport(convergencePath,convergenceReport); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write convergence report: %v",err) }
	if err:=writeSlowTickReport(slowTickReportPath(convergencePath),convergenceSlowReport); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write convergence slow tick report: %v",err) }
	log.Printf("semantic convergence reached: %.3fs tick=%d desired=%d reliable_queued=%d reliable_in_flight=%d", convergenceMeta.ReadyToConvergedSeconds, convergenceMeta.Observation.Tick, convergenceMeta.Observation.World.DesiredRelationships, convergenceMeta.Reliable.Queued, convergenceMeta.Reliable.InFlight)

	latestMeta := convergenceMeta
	// Phase 2（teleport-churn only）：從已收斂世界一次性交換半數 Entity 的 cluster membership。
	// Gate 必須先看到 non-converged，再等 Spawn/Despawn/Vitals + Reliable drain 重新成立。
	if scenario == loadlab.ScenarioTeleportChurn {
		collector.Reset(); slowTicks.Reset(); server.ResetNetworkMetrics(); convergence.Start()
		churnStarted := time.Now()
		if err := worldRuntime.EnqueueTeleportBatch(churnRequests); err != nil {
			convergence.Stop(); stop(); <-serveDone; <-loopDone; log.Fatalf("enqueue teleport churn batch: %v", err)
		}
		log.Printf("teleport churn triggered: moved=%d timeout=%s stable=%s", len(churnRequests), churnTimeout.String(), churnStable.String())
		churnMeta, churnErr := waitForTransitionConvergence(ctx, convergence, server, *clients, *churnTimeout, *churnStable, churnStarted, "teleport-churn")
		convergence.Stop()
		if churnErr != nil { stop(); <-serveDone; <-loopDone; log.Fatal(churnErr) }
		latestMeta = churnMeta
		churnReport := withPhaseReport(withNetworkMetrics(collector.Finish(scenario,*clients), server.NetworkMetrics()), "churn", churnMeta)
		churnSlowReport := slowTicks.Finish()
		churnPath := churnReportPath(*reportPath)
		if err:=loadlab.WriteReport(churnPath,churnReport); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write churn report: %v",err) }
		if err:=writeSlowTickReport(slowTickReportPath(churnPath),churnSlowReport); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write churn slow tick report: %v",err) }
		log.Printf("teleport churn converged: %.3fs tick=%d desired=%d spawn_selected=%d despawn_selected=%d backpressure_stops=%d reliable_queued=%d reliable_in_flight=%d", churnMeta.TriggerToConvergedSeconds, churnMeta.Observation.Tick, churnMeta.Observation.World.DesiredRelationships, churnReport.Lifecycle.SpawnSelected, churnReport.Lifecycle.DespawnSelected, churnReport.Lifecycle.BackpressureStops, churnMeta.Reliable.Queued, churnMeta.Reliable.InFlight)
	}

	// Final phase：capacity / allocation 數據只量最後一次 semantic convergence 之後的 steady-state。
	if err := profiler.Write("before"); err != nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write allocation profile before measurement: %v", err) }
	collector.Reset(); slowTicks.Reset(); server.ResetNetworkMetrics()
	log.Printf("starting %s steady-state measurement window", duration.String())

	measurementTimer:=time.NewTimer(*duration); completed:=false
	select { case <-measurementTimer.C: completed=true; case <-ctx.Done(): measurementTimer.Stop() }
	report:=withPhaseReport(withNetworkMetrics(collector.Finish(scenario,*clients), server.NetworkMetrics()), "steady-state", latestMeta)
	slowReport:=slowTicks.Finish()
	if err := profiler.Write("after"); err != nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write allocation profile after measurement: %v", err) }
	if err:=loadlab.WriteReport(*reportPath,report); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write report: %v",err) }
	if err:=writeSlowTickReport(slowTickReportPath(*reportPath),slowReport); err!=nil { stop(); <-serveDone; <-loopDone; log.Fatalf("write slow tick report: %v",err) }
	log.Printf("steady-state report written: %s ticks=%d p99=%.3fms max_queue=%d datagram_too_large=%d realtime_mbps=%.3f encode_avg_us=%.3f",*reportPath,report.Ticks,report.TickDuration.P99MS,report.Queue.MaxDepthBefore,report.Errors.DatagramTooLarge,report.Network.RealtimeMbitsPerSec,report.Network.EncodeAverageUS)

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
