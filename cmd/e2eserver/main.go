// Command e2eserver is the S4-E.1 CI-only real-process server harness.
// It composes the production transport/runtime/gameplay packages while deliberately
// keeping deterministic test identity assignment out of cmd/worldd.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/siegeownership"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	e2eAttackerCharacterID = "e2e-attacker"
	e2eDefenderCharacterID = "e2e-defender"
)

func main() {
	var (
		tcpAddress     = flag.String("tcp", "127.0.0.1:27777", "Reliable TCP listen address; loopback only")
		udpAddress     = flag.String("udp", "127.0.0.1:27778", "Realtime UDP listen address; loopback only")
		tickRate       = flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
		snapshotRate   = flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
		worldPath      = flag.String("world", "worlds/castle-sandbox/gameplay.json", "Gameplay World JSON path")
		combatPath     = flag.String("combat-actions", "config/combat-actions.json", "Combat Action Catalog JSON path")
		siegeMatchPath = flag.String("siege-match", "testdata/s4e/siege-match.json", "S4-E.1 Siege Match fixture")
		respawnPath    = flag.String("respawn-policy", "testdata/s4e/respawn-policy.json", "S4-E.1 respawn fixture")
		ownershipDir   = flag.String("siege-ownership-dir", "artifacts/s4e1-ownership", "Durable S4-E.1 castle ownership directory")
	)
	flag.Parse()

	if err := validateHarnessAddress(*tcpAddress); err != nil {
		log.Fatal(err)
	}
	if err := validateHarnessAddress(*udpAddress); err != nil {
		log.Fatal(err)
	}
	if *tickRate <= 0 || *snapshotRate <= 0 || *snapshotRate > *tickRate || *tickRate%*snapshotRate != 0 {
		log.Fatal("tick-rate must be positive and evenly divisible by snapshot-rate")
	}

	loadedWorld, err := gameplayworld.LoadFile(*worldPath)
	if err != nil {
		log.Fatalf("load gameplay world %q: %v", *worldPath, err)
	}
	loadedSiege, err := siege.LoadMatchFile(*siegeMatchPath)
	if err != nil {
		log.Fatalf("load siege match %q: %v", *siegeMatchPath, err)
	}
	if err := siege.ValidateMatchAgainstGates(loadedSiege.Definition, loadedWorld.Definition.Gates); err != nil {
		log.Fatalf("validate siege match against gameplay world: %v", err)
	}
	loadedCombat, err := combat.LoadFile(*combatPath)
	if err != nil {
		log.Fatalf("load combat actions %q: %v", *combatPath, err)
	}
	combatService, err := combat.NewService(loadedCombat.Definition.Actions)
	if err != nil {
		log.Fatalf("build combat service: %v", err)
	}
	loadedRespawn, err := respawnpolicy.LoadFile(*respawnPath)
	if err != nil {
		log.Fatalf("load respawn policy %q: %v", *respawnPath, err)
	}
	if err := respawnpolicy.ValidateAgainstWorld(loadedRespawn.Definition, loadedWorld.Definition); err != nil {
		log.Fatalf("validate respawn policy against gameplay world: %v", err)
	}
	respawnService, err := respawnpolicy.NewService(loadedRespawn.Definition, *tickRate)
	if err != nil {
		log.Fatalf("build respawn service: %v", err)
	}

	ownershipStore, err := siegeownership.Open(*ownershipDir)
	if err != nil {
		log.Fatalf("open siege ownership store %q: %v", *ownershipDir, err)
	}
	ownership, _, err := ownershipStore.LoadOrCreate(
		loadedWorld.Definition.WorldID,
		loadedSiege.Definition.DefenderID,
	)
	if err != nil {
		log.Fatalf("load/create siege ownership: %v", err)
	}
	commitOwnership := func(transfer siege.CastleOwnershipTransfer) (siege.CastleOwnershipState, error) {
		return ownershipStore.Commit(loadedWorld.Definition.WorldID, transfer)
	}

	navigator, err := navigation.NewGameplayNavigator(loadedWorld.Definition)
	if err != nil {
		log.Fatalf("build gameplay navigator: %v", err)
	}
	moveService := movement.NewService(navigator, 0.1)
	simulationWorld := simulation.New(spatial.NewGrid(32), moveService)
	runtimeConfig := worldruntime.DefaultConfig()
	runtimeConfig.SnapshotEveryTicks = uint64(*tickRate / *snapshotRate)
	worldRuntime := worldruntime.New(
		simulationWorld,
		runtimeConfig,
		worldruntime.WithDynamicWorld(navigator),
		worldruntime.WithSiegeGates(loadedWorld.Definition.Gates),
		worldruntime.WithSiegeMatch(loadedSiege.Definition),
		worldruntime.WithSiegeOwnershipPersistence(ownership, commitOwnership),
		worldruntime.WithCombatService(combatService),
		worldruntime.WithRespawnPolicy(respawnService),
	)
	loop, err := worldruntime.NewLoop(worldRuntime, *tickRate)
	if err != nil {
		log.Fatal(err)
	}

	worldIdentity := protocol.WorldIdentity{
		WorldID:        loadedWorld.Definition.WorldID,
		Revision:       loadedWorld.Definition.Revision,
		GameplaySHA256: loadedWorld.SHA256,
	}
	networkConfig := tcpudp.DefaultConfig()
	networkConfig.TCPAddress = *tcpAddress
	networkConfig.UDPAddress = *udpAddress
	networkConfig.TickRateHz = uint16(*tickRate)
	networkConfig.SnapshotRateHz = uint16(*snapshotRate)
	networkConfig.WorldIdentity = worldIdentity
	networkConfig.PlayerFactory = e2ePlayerFactory
	networkConfig.CharacterIdentityFactory = e2eCharacterIdentityFactory
	server := tcpudp.NewServer(networkConfig, worldRuntime, gamev1.Codec{})
	if err := server.Open(); err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	loopDone := make(chan error, 1)
	serverDone := make(chan error, 1)
	var roundTwoOnce sync.Once
	go func() {
		loopDone <- loop.RunObserved(ctx, func(report worldruntime.StepReport) {
			for _, commandErr := range report.CommandErrors {
				log.Printf("S4-E.1 command error tick=%d command=%s session=%d err=%v", report.Tick, commandErr.Command, commandErr.SessionID, commandErr.Err)
			}
			observeRoundTwoDurability(worldRuntime, ownershipStore, loadedWorld.Definition.WorldID, &roundTwoOnce)
		})
	}()
	go func() { serverDone <- server.Serve(ctx) }()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case networkErr := <-server.Errors():
				log.Printf("S4-E.1 network error session=%d operation=%s err=%v", networkErr.SessionID, networkErr.Operation, networkErr.Err)
			}
		}
	}()

	log.Printf(
		"ASTRAHOLD_E2E_SERVER_READY protocol=%d tcp=%s udp=%s world=%s@%s siege=%s respawn=%s",
		protocol.Version,
		server.TCPAddr(),
		server.UDPAddr(),
		worldIdentity.WorldID,
		worldIdentity.Revision,
		loadedSiege.Revision,
		loadedRespawn.Definition.Revision,
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
			log.Fatalf("serve: %v", err)
		}
		stop()
	}
}

func validateHarnessAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("S4-E.1 harness only permits loopback listen addresses: %q", address)
	}
	return nil
}

func e2eCharacterIdentityFactory(sessionID session.ID, _ world.EntityID) (characteridentity.Binding, error) {
	var characterID string
	switch sessionID {
	case 1:
		characterID = e2eAttackerCharacterID
	case 2:
		characterID = e2eDefenderCharacterID
	default:
		return characteridentity.Binding{}, fmt.Errorf("S4-E.1 supports exactly two sessions: %d", sessionID)
	}
	return characteridentity.NewTrusted(characterID)
}

func e2ePlayerFactory(sessionID session.ID, entityID world.EntityID) tcpudp.PlayerSpec {
	x := float32(0)
	if sessionID == 2 {
		x = 1.5
	}
	return tcpudp.PlayerSpec{
		Entity: world.EntityState{
			ID:        entityID,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{X: x, Layer: 0}},
		},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
		AOIRadius:     64,
	}
}

func observeRoundTwoDurability(runtime *worldruntime.Runtime, store *siegeownership.Store, worldID string, once *sync.Once) {
	match, matchOK := runtime.SiegeMatchState()
	ownership, ownershipOK := runtime.SiegeCastleOwnershipState()
	if !matchOK || !ownershipOK || match.Round != 2 || match.Phase != siege.MatchPhaseGate ||
		match.AttackerID != "defenders" || match.DefenderID != "attackers" ||
		ownership.OwnerID != "attackers" || ownership.Revision != 2 {
		return
	}
	durable, exists, err := store.Load(worldID)
	if err != nil || !exists || durable != ownership {
		if err != nil {
			log.Printf("S4-E.1 durable ownership verify failed: %v", err)
		}
		return
	}
	once.Do(func() {
		log.Printf(
			"ASTRAHOLD_E2E_SERVER_OK round=%d attacker=%s defender=%s owner=%s ownership_revision=%d",
			match.Round,
			match.AttackerID,
			match.DefenderID,
			ownership.OwnerID,
			ownership.Revision,
		)
	})
}
