// Command browserwse2e is a loopback-only BrowserWS integration harness for Protocol v27.
// It composes the real BrowserWS adapter, game codec and authoritative worldruntime while
// keeping deterministic trusted identity/class bootstrap out of normal cmd/worldd behavior.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/browserws"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	e2eCharacterID = "e2e-browser-shadowblade"
	e2eTargetID    = world.EntityID(9603)
)

func main() {
	listenAddress := flag.String("browser-ws", "127.0.0.1:27779", "BrowserWS listen address; loopback only")
	tickRate := flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
	snapshotRate := flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
	flag.Parse()

	if err := validateHarnessAddress(*listenAddress); err != nil {
		log.Fatal(err)
	}
	if *tickRate <= 0 || *snapshotRate <= 0 || *snapshotRate > *tickRate || *tickRate%*snapshotRate != 0 {
		log.Fatal("tick-rate must be positive and evenly divisible by snapshot-rate")
	}

	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "browserws-target-resource-e2e",
		Revision:      "v27-shadowblade-flaw-001",
		Units:         "meters",
		Agent: gameplayworld.AgentDefaults{
			Radius:        0.35,
			Height:        1.8,
			MaxStepHeight: 0.5,
		},
		Surfaces: []gameplayworld.Surface{{
			ID:    "ground",
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{
				MinX: -20,
				MaxX: 20,
				MinZ: -20,
				MaxZ: 20,
			},
		}},
	}
	navigator, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		log.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:              classaction.ShadowbladeDualBladeStrike,
		Effect:          combat.EffectDamage,
		Targets:         []combat.TargetKind{combat.TargetEntity},
		Range:           4.5,
		BaseDamage:      90,
		DamageType:      combat.DamagePhysical,
		Blockable:       true,
		CooldownSeconds: 0.95,
	}})
	if err != nil {
		log.Fatal(err)
	}

	worldIdentity := protocol.WorldIdentity{
		WorldID:        definition.WorldID,
		Revision:       definition.Revision,
		GameplaySHA256: strings.Repeat("b", 64),
	}
	stateWorld := characterstate.WorldRef{
		WorldID:        worldIdentity.WorldID,
		Revision:       worldIdentity.Revision,
		GameplaySHA256: worldIdentity.GameplaySHA256,
	}
	stateOutbox, err := characterstate.NewOutbox(32)
	if err != nil {
		log.Fatal(err)
	}

	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigator, 0.1))
	config := worldruntime.DefaultConfig()
	config.SnapshotEveryTicks = uint64(*tickRate / *snapshotRate)
	targetSpawn := worldruntime.SpawnEntityRequest{
		Entity: world.EntityState{
			ID:          e2eTargetID,
			Kind:        world.EntityMonster,
			ArchetypeID: "e2e-flaw-target",
			Transform: world.Transform{
				Position: world.Position{X: 3, Layer: 0},
				Yaw:      0,
			},
		},
		Speed:         1,
		Radius:        0.35,
		MaxStepHeight: 0.5,
		HP:            180,
		MaxHP:         180,
	}
	runtime := worldruntime.New(
		sim,
		config,
		worldruntime.WithDynamicWorld(navigator),
		worldruntime.WithCombatService(combatService),
		worldruntime.WithCharacterStateOutbox(stateOutbox, stateWorld),
		worldruntime.WithMonsterLifecycle(worldruntime.MonsterLifecycleConfig{
			Spawn:             targetSpawn,
			CorpseHoldTicks:   1,
			RespawnDelayTicks: uint64(*tickRate * 60),
		}),
	)
	if err := runtime.EnqueueSpawnEntity(targetSpawn); err != nil {
		log.Fatalf("queue E2E target spawn: %v", err)
	}
	loop, err := worldruntime.NewLoop(runtime, *tickRate)
	if err != nil {
		log.Fatal(err)
	}

	trustedIdentity, err := characteridentity.NewTrusted(e2eCharacterID)
	if err != nil {
		log.Fatal(err)
	}
	browserConfig := browserws.DefaultConfig()
	browserConfig.TickRateHz = uint16(*tickRate)
	browserConfig.SnapshotRateHz = uint16(*snapshotRate)
	browserConfig.WorldIdentity = worldIdentity
	browserConfig.OriginPatterns = []string{"http://127.0.0.1:*", "http://localhost:*"}
	browserConfig.PlayerFactory = func(_ session.ID, entityID world.EntityID) browserws.PlayerSpec {
		return browserws.PlayerSpec{
			Entity: world.EntityState{
				ID:        entityID,
				Kind:      world.EntityPlayer,
				Transform: world.Transform{Position: world.Position{Layer: 0}},
			},
			Speed:         6,
			Radius:        0.35,
			MaxStepHeight: 0.5,
			AOIRadius:     64,
		}
	}
	browserConfig.TrustedE2EBootstrapFactory = func(sessionID session.ID, _ world.EntityID) (browserws.TrustedE2EBootstrap, error) {
		if sessionID != 1 {
			return browserws.TrustedE2EBootstrap{}, fmt.Errorf("browserwse2e supports exactly one session: %d", sessionID)
		}
		return browserws.TrustedE2EBootstrap{
			Identity: trustedIdentity,
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID:   trustedIdentity.ID,
				Revision:      1,
				World:         worldIdentity,
				ClassID:       classid.Shadowblade,
				HP:            1000,
				MaxHP:         1000,
				MP:            100,
				MaxMP:         100,
				Transform:     world.Transform{Position: world.Position{Layer: 0}},
				Inventory:     characterstate.InventoryState{Initialized: true},
			},
		}, nil
	}

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", browserws.NewHandler(browserConfig, runtime, gamev1.Codec{}))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	loopDone := make(chan error, 1)
	serverDone := make(chan error, 1)
	go func() {
		loopDone <- loop.RunObserved(ctx, func(report worldruntime.StepReport) {
			for _, commandErr := range report.CommandErrors {
				log.Printf("browserwse2e command error tick=%d command=%s session=%d err=%v", report.Tick, commandErr.Command, commandErr.SessionID, commandErr.Err)
			}
			for _, rejection := range report.ActionRejections {
				log.Printf("browserwse2e action rejected tick=%d action=%s session=%d err=%v", report.Tick, rejection.Action, rejection.SessionID, rejection.Err)
			}
		})
	}()
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverDone <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf(
		"ASTRAHOLD_BROWSERWS_E2E_READY protocol=%d ws=ws://%s/ws character=%s class=%s target=%d action=%s contract=first-hit-flaw-current-positive-second-hit-defeat-then-despawn-clear",
		protocol.Version,
		listener.Addr().String(),
		e2eCharacterID,
		classid.Shadowblade,
		e2eTargetID,
		classaction.ShadowbladeDualBladeStrike,
	)

	select {
	case <-ctx.Done():
	case err := <-loopDone:
		if err != nil {
			log.Fatalf("world loop: %v", err)
		}
		stop()
	case err := <-serverDone:
		if err != nil {
			log.Fatalf("browserws serve: %v", err)
		}
		stop()
	}
}

func validateHarnessAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("browserwse2e only permits loopback listen addresses: %q", address)
	}
	return nil
}
