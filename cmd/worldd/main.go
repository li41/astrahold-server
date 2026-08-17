package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/deathpenalty"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	deathOutcomeDrainEvery = 100 * time.Millisecond
	deathOutcomeDrainBatch = 64
)

func main() {
	var (
		tcpAddress                       = flag.String("tcp", "127.0.0.1:7777", "Reliable TCP listen address")
		udpAddress                       = flag.String("udp", "127.0.0.1:7778", "Realtime UDP listen address")
		tickRate                         = flag.Int("tick-rate", 20, "World simulation tick rate (Hz)")
		snapshotRate                     = flag.Int("snapshot-rate", 10, "Network snapshot rate (Hz)")
		worldPath                        = flag.String("world", "worlds/castle-sandbox/gameplay.json", "Gameplay World JSON path")
		combatPath                       = flag.String("combat-actions", "config/combat-actions.json", "Combat Action Catalog JSON path")
		siegeMatchPath                   = flag.String("siege-match", "config/siege-match.json", "Authoritative Siege Match config JSON path")
		respawnPolicyPath                = flag.String("respawn-policy", "config/respawn-policy.json", "Server respawn policy JSON path")
		deathPenaltyPath                 = flag.String("death-penalty", "config/death-penalty.json", "Server death penalty policy JSON path")
		deathOutcomeOutboxCapacity       = flag.Int("death-outcome-outbox-capacity", 4096, "Process-local death outcome outbox capacity")
		deathOutcomeJournalPath          = flag.String("death-outcome-journal", "data/death-outcomes.journal", "Durable append-only death outcome journal path")
		deathOutcomeCheckpointPath       = flag.String("death-outcome-checkpoint", "data/death-outcomes.checkpoint.json", "Durable death outcome consumer checkpoint path")
		characterStateOutboxCapacity     = flag.Int("character-state-outbox-capacity", 4096, "Process-local trusted character state save outbox capacity")
		characterStateDir                = flag.String("character-state-dir", "data/character-state", "Durable trusted character state directory")
		characterStateSaveJournalPath    = flag.String("character-state-save-journal", "data/character-state-saves.journal", "Durable append-only trusted character state save-intent journal path")
		characterStateSaveCheckpointPath = flag.String("character-state-save-checkpoint", "data/character-state-saves.checkpoint.json", "Durable trusted character state save consumer checkpoint path")
		postReviveProtectionSeconds      = flag.Float64("post-revive-protection-seconds", 3.0, "Server-side damage protection after respawn/resurrection; 0 disables")
	)
	flag.Parse()
	if err := validateRates(*tickRate, *snapshotRate); err != nil {
		log.Fatal(err)
	}
	trustedCharacterAuthenticator, trustedCharacterAuthRuntime, trustedCharacterAuthRevision, err := loadRuntimeTrustedCharacterAuthenticator(*trustedCharacterAuthFile, *tcpAddress)
	if err != nil {
		log.Fatal(err)
	}
	trustedTLSConfig, err := loadTrustedTLSIngressConfig(*trustedTLSListen, *trustedTLSCertFile, *trustedTLSKeyFile, *tcpAddress, trustedCharacterAuthenticator != nil)
	if err != nil {
		log.Fatal(err)
	}
	protectionTicks, err := reviveProtectionTicks(*postReviveProtectionSeconds, *tickRate)
	if err != nil {
		log.Fatal(err)
	}
	deathOutbox, err := deathoutcome.NewOutbox(*deathOutcomeOutboxCapacity)
	if err != nil {
		log.Fatalf("build death outcome outbox: %v", err)
	}
	deathJournal, err := deathoutcome.OpenJournal(*deathOutcomeJournalPath)
	if err != nil {
		log.Fatalf("open death outcome journal %q: %v", *deathOutcomeJournalPath, err)
	}
	defer func() {
		if err := deathJournal.Close(); err != nil {
			log.Printf("close death outcome journal: %v", err)
		}
	}()
	deathCheckpointStore, err := deathoutcome.NewCheckpointStore(*deathOutcomeCheckpointPath)
	if err != nil {
		log.Fatalf("build death outcome checkpoint store %q: %v", *deathOutcomeCheckpointPath, err)
	}
	deathCheckpoint, recoveredDeathOutcomes, err := recoverDeathOutcomeJournal(deathJournal, deathCheckpointStore, logDeathOutcomeJournalRecord)
	if err != nil {
		log.Fatalf("recover death outcome journal: %v", err)
	}
	if deathJournal.RepairedTail() {
		log.Printf("death outcome journal repaired incomplete crash tail: path=%s last_record_id=%d", deathJournal.Path(), deathJournal.LastRecordID())
	}

	characterStateOutbox, err := characterstate.NewOutbox(*characterStateOutboxCapacity)
	if err != nil {
		log.Fatalf("build character state outbox: %v", err)
	}
	characterStateStore, err := characterstate.Open(*characterStateDir)
	if err != nil {
		log.Fatalf("open character state store %q: %v", *characterStateDir, err)
	}
	characterStateSaveJournal, err := characterstate.OpenSaveJournal(*characterStateSaveJournalPath)
	if err != nil {
		log.Fatalf("open character state save journal %q: %v", *characterStateSaveJournalPath, err)
	}
	defer func() {
		if err := characterStateSaveJournal.Close(); err != nil {
			log.Printf("close character state save journal: %v", err)
		}
	}()
	characterStateSaveCheckpointStore, err := characterstate.NewSaveCheckpointStore(*characterStateSaveCheckpointPath)
	if err != nil {
		log.Fatalf("build character state save checkpoint store %q: %v", *characterStateSaveCheckpointPath, err)
	}
	characterStateSaveCheckpoint, recoveredCharacterStateSaves, err := recoverCharacterStateSaveJournal(characterStateSaveJournal, characterStateSaveCheckpointStore, characterStateStore)
	if err != nil {
		log.Fatalf("recover character state save journal: %v", err)
	}
	if characterStateSaveJournal.RepairedTail() {
		log.Printf("character state save journal repaired incomplete crash tail: path=%s last_record_id=%d", characterStateSaveJournal.Path(), characterStateSaveJournal.LastRecordID())
	}

	loadedWorld, err := gameplayworld.LoadFile(*worldPath)
	if err != nil {
		log.Fatalf("load gameplay world %q: %v", *worldPath, err)
	}
	loadedSiegeMatch, err := siege.LoadMatchFile(*siegeMatchPath)
	if err != nil {
		log.Fatalf("load siege match %q: %v", *siegeMatchPath, err)
	}
	if err := siege.ValidateMatchAgainstGates(loadedSiegeMatch.Definition, loadedWorld.Definition.Gates); err != nil {
		log.Fatalf("validate siege match %q against gameplay world: %v", *siegeMatchPath, err)
	}
	siegeOwnershipPersistence, siegeOwnership, siegeOwnershipCreated, err := openSiegeOwnershipPersistence(*siegeOwnershipDir, loadedWorld.Definition.WorldID, loadedSiegeMatch.Definition.DefenderID)
	if err != nil {
		log.Fatalf("open siege ownership store %q for world %q: %v", *siegeOwnershipDir, loadedWorld.Definition.WorldID, err)
	}
	loadedCombat, err := combat.LoadFile(*combatPath)
	if err != nil {
		log.Fatalf("load combat actions %q: %v", *combatPath, err)
	}
	combatService, err := combat.NewService(loadedCombat.Definition.Actions)
	if err != nil {
		log.Fatalf("build combat service: %v", err)
	}
	loadedRespawnPolicy, err := respawnpolicy.LoadFile(*respawnPolicyPath)
	if err != nil {
		log.Fatalf("load respawn policy %q: %v", *respawnPolicyPath, err)
	}
	if err := respawnpolicy.ValidateAgainstWorld(loadedRespawnPolicy.Definition, loadedWorld.Definition); err != nil {
		log.Fatalf("validate respawn policy %q against gameplay world: %v", *respawnPolicyPath, err)
	}
	respawnService, err := respawnpolicy.NewService(loadedRespawnPolicy.Definition, *tickRate)
	if err != nil {
		log.Fatalf("build respawn policy service: %v", err)
	}
	freshSpawn, err := freshPlayerSpawn(loadedRespawnPolicy.Definition)
	if err != nil {
		log.Fatalf("resolve fresh player spawn from respawn policy %q: %v", *respawnPolicyPath, err)
	}
	loadedDeathPenalty, err := deathpenalty.LoadFile(*deathPenaltyPath)
	if err != nil {
		log.Fatalf("load death penalty policy %q: %v", *deathPenaltyPath, err)
	}
	deathPenaltyService, err := deathpenalty.NewService(loadedDeathPenalty.Definition)
	if err != nil {
		log.Fatalf("build death penalty service: %v", err)
	}
	pveRespawnDelay, _ := respawnService.DelayTicks(respawnpolicy.DeathContextPvE)
	pvpRespawnDelay, _ := respawnService.DelayTicks(respawnpolicy.DeathContextPvP)
	siegeRespawnDelay, _ := respawnService.DelayTicks(respawnpolicy.DeathContextSiege)
	nav, err := navigation.NewGameplayNavigator(loadedWorld.Definition)
	if err != nil {
		log.Fatalf("build gameplay navigator: %v", err)
	}

	move := movement.NewService(nav, 0.1)
	sim := simulation.New(spatial.NewGrid(32), move)
	runtimeConfig := worldruntime.DefaultConfig()
	runtimeConfig.SnapshotEveryTicks = uint64(*tickRate / *snapshotRate)
	runtimeConfig.PostReviveProtectionTicks = protectionTicks
	autosaveTicks, err := configureCharacterStateAutosave(&runtimeConfig, *characterStateAutosaveSeconds, *characterStateAutosavesPerTick, *tickRate)
	if err != nil {
		log.Fatal(err)
	}
	worldIdentity := protocol.WorldIdentity{
		WorldID: loadedWorld.Definition.WorldID, Revision: loadedWorld.Definition.Revision, GameplaySHA256: loadedWorld.SHA256,
	}
	characterStateWorld := characterstate.WorldRef{
		WorldID: worldIdentity.WorldID, Revision: worldIdentity.Revision, GameplaySHA256: worldIdentity.GameplaySHA256,
	}
	characterStatePersistence := newCharacterStatePersistence(
		characterStateOutbox,
		characterStateSaveJournal,
		characterStateSaveCheckpointStore,
		characterStateSaveCheckpoint,
		characterStateStore,
		worldIdentity,
	)
	runtime := worldruntime.New(
		sim,
		runtimeConfig,
		worldruntime.WithDynamicWorld(nav),
		worldruntime.WithSiegeGates(loadedWorld.Definition.Gates),
		worldruntime.WithSiegeMatch(loadedSiegeMatch.Definition),
		worldruntime.WithSiegeOwnershipPersistence(siegeOwnership, siegeOwnershipPersistence.Commit),
		worldruntime.WithCombatService(combatService),
		worldruntime.WithRespawnPolicy(respawnService),
		worldruntime.WithDeathPenalty(deathPenaltyService),
		worldruntime.WithDeathOutcomeOutbox(deathOutbox),
		worldruntime.WithCharacterStateOutbox(characterStateOutbox, characterStateWorld),
	)
	loop, err := worldruntime.NewLoop(runtime, *tickRate)
	if err != nil {
		log.Fatal(err)
	}

	networkConfig := tcpudp.DefaultConfig()
	networkConfig.TCPAddress = *tcpAddress
	networkConfig.UDPAddress = *udpAddress
	networkConfig.TickRateHz = uint16(*tickRate)
	networkConfig.SnapshotRateHz = uint16(*snapshotRate)
	networkConfig.WorldIdentity = worldIdentity
	networkConfig.PlayerFactory = newWorldPlayerFactory(freshSpawn, loadedWorld.Definition.Agent)
	networkConfig.CharacterRestoreFactory = characterStatePersistence.LoadRestore
	if trustedCharacterAuthenticator != nil {
		networkConfig.TrustedCharacterConnectionAuthenticator = trustedCharacterAuthenticator
	}
	server := tcpudp.NewServer(networkConfig, runtime, gamev1.Codec{})
	if trustedCharacterAuthRuntime != nil {
		initialScopes := activeTrustedCharacterAuthenticationScopes(trustedCharacterAuthRuntime.provider.snapshot(), time.Now().UTC())
		server.ReplaceTrustedCharacterAuthenticationScopes(initialScopes)
	}
	if err := server.Open(); err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	var tlsIngress *trustedTLSIngress
	if trustedTLSConfig != nil {
		tlsIngress, err = openTrustedTLSIngress(trustedTLSConfig)
		if err != nil {
			log.Fatal(err)
		}
		defer tlsIngress.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if trustedCharacterAuthRuntime != nil {
		reloadSignals := make(chan os.Signal, 1)
		signal.Notify(reloadSignals, syscall.SIGHUP)
		defer signal.Stop(reloadSignals)
		go runTrustedCharacterAuthRuntime(ctx, reloadSignals, trustedCharacterAuthRuntime, server.ReplaceTrustedCharacterAuthenticationScopes, log.Printf)
	}
	if tlsIngress != nil {
		go func() {
			if err := tlsIngress.Serve(ctx); err != nil {
				log.Printf("trusted TLS ingress stopped with error: %v", err)
				stop()
			}
		}()
	}
	loopDone := make(chan error, 1)
	go func() { loopDone <- loop.RunObserved(ctx, logStepReport) }()
	go logNetworkErrors(ctx, server.Errors())

	journalCtx, stopJournal := context.WithCancel(context.Background())
	journalDone := make(chan error, 1)
	go func() {
		err := runDeathOutcomeJournal(journalCtx, deathOutbox, deathJournal, deathCheckpointStore, deathCheckpoint, logDeathOutcomeJournalRecord)
		if err != nil {
			log.Printf("death outcome journal worker stopped with error: %v", err)
			stop()
		}
		journalDone <- err
	}()

	characterStateCtx, stopCharacterState := context.WithCancel(context.Background())
	characterStateDone := make(chan error, 1)
	go func() {
		err := runCharacterStateStore(characterStateCtx, characterStatePersistence)
		if err != nil {
			log.Printf("character state persistence worker stopped with error: %v", err)
			stop()
		}
		characterStateDone <- err
	}()

	log.Printf("Astrahold worldd ready: protocol=%d world=%s revision=%s gameplay_sha256=%s combat_revision=%s actions=%d siege_match_revision=%s siege_match=%s attacker=%s defender=%s breach_gate=%s throne=%s respawn_revision=%s respawn_pve_delay_ticks=%d respawn_pvp_delay_ticks=%d respawn_siege_delay_ticks=%d death_penalty_revision=%s checkpoint_forfeit_pve=%t checkpoint_forfeit_pvp=%t checkpoint_forfeit_siege=%t death_outcome_outbox_capacity=%d death_outcome_journal_id=%s death_outcome_journal_last_record=%d death_outcome_checkpoint_record=%d death_outcome_recovered_records=%d character_state_save_journal_id=%s character_state_save_journal_last_record=%d character_state_save_checkpoint_record=%d character_state_save_recovered_records=%d character_state_autosave_ticks=%d character_state_autosaves_per_tick=%d post_revive_protection_ticks=%d spawn_points=%d tcp=%s udp=%s tick_rate=%dHz snapshot_rate=%dHz codec=gamev1 gates=%d", protocol.Version, loadedWorld.Definition.WorldID, loadedWorld.Definition.Revision, loadedWorld.SHA256[:12], loadedCombat.Definition.Revision, len(loadedCombat.Definition.Actions), loadedSiegeMatch.Revision, loadedSiegeMatch.Definition.ID, loadedSiegeMatch.Definition.AttackerID, loadedSiegeMatch.Definition.DefenderID, loadedSiegeMatch.Definition.BreachGateID, loadedSiegeMatch.Definition.ThroneObjectiveID, respawnService.Revision(), pveRespawnDelay, pvpRespawnDelay, siegeRespawnDelay, deathPenaltyService.Revision(), deathPenaltyService.ForfeitsCheckpoint(respawnpolicy.DeathContextPvE), deathPenaltyService.ForfeitsCheckpoint(respawnpolicy.DeathContextPvP), deathPenaltyService.ForfeitsCheckpoint(respawnpolicy.DeathContextSiege), deathOutbox.Capacity(), deathJournal.ID(), deathJournal.LastRecordID(), deathCheckpoint.RecordID, recoveredDeathOutcomes, characterStateSaveJournal.ID(), characterStateSaveJournal.LastRecordID(), characterStateSaveCheckpoint.RecordID, recoveredCharacterStateSaves, autosaveTicks, *characterStateAutosavesPerTick, protectionTicks, respawnService.SpawnPointCount(), server.TCPAddr(), server.UDPAddr(), *tickRate, *snapshotRate, len(loadedWorld.Definition.Gates))
	log.Printf("death outcome durability: journal=%s checkpoint=%s append_fsync=true checkpoint_atomic_rename=true", deathJournal.Path(), deathCheckpointStore.Path())
	log.Printf("character state durability: dir=%s outbox_capacity=%d trusted_only=true optimistic_revision=true atomic_rename=true save_journal=%s save_checkpoint=%s journal_append_fsync=true checkpoint_atomic_rename=true startup_recovery=true restore_exact_world=true defeated_restore=true autosave_ticks=%d autosaves_per_tick=%d autosave_capture_process_local=true", characterStateStore.Path(), characterStateOutbox.Capacity(), characterStateSaveJournal.Path(), characterStateSaveCheckpointStore.Path(), autosaveTicks, *characterStateAutosavesPerTick)
	log.Printf("siege ownership durability: world=%s dir=%s revision=%d owner=%s previous_owner=%s last_transfer_match=%s created=%t single_writer=true optimistic_revision=true temp_fsync=true atomic_rename=true directory_fsync=true startup_recovery=true completion_barrier=true", loadedWorld.Definition.WorldID, siegeOwnershipPersistence.Path(), siegeOwnership.Revision, siegeOwnership.OwnerID, siegeOwnership.PreviousOwnerID, siegeOwnership.LastTransferMatchID, siegeOwnershipCreated)
	if trustedCharacterAuthenticator != nil {
		log.Printf("trusted character authentication: enabled=true revision=%s identity_source=server_credential_map pre_gamev1=true tcp_loopback_required=true takeover_authorizer=credential_scoped_optional runtime_reload=%s", trustedCharacterAuthRevision, describeTrustedCharacterAuthRuntime(trustedCharacterAuthRuntime))
	} else {
		log.Printf("trusted character authentication: enabled=false identity_source=ephemeral_default")
	}
	if tlsIngress != nil {
		log.Printf("trusted TLS ingress: enabled=true listen=%s upstream=%s min_tls=1.3 credential_transport=tls", tlsIngress.Addr(), *tcpAddress)
		log.Printf("realtime UDP remains GameV1 token-authenticated and is not encrypted")
	} else {
		log.Printf("trusted TLS ingress: enabled=false")
		log.Printf("development transport is for local/controlled environments; do not expose it directly to the Internet")
	}
	if err := server.Serve(ctx); err != nil {
		stop()
		log.Printf("network server stopped with error: %v", err)
	}
	if err := <-loopDone; err != nil {
		log.Printf("world loop stopped with error: %v", err)
	}

	// Stop persistence workers only after the world loop is done producing new events/intents.
	// Each worker drains its process-local outbox and durable journal before exit.
	stopCharacterState()
	if err := <-characterStateDone; err != nil {
		log.Printf("character state persistence shutdown error: %v", err)
	}
	stopJournal()
	if err := <-journalDone; err != nil {
		log.Printf("death outcome journal shutdown error: %v", err)
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

func reviveProtectionTicks(seconds float64, tickRate int) (uint64, error) {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("post-revive-protection-seconds must be finite and >= 0")
	}
	if seconds == 0 {
		return 0, nil
	}
	ticks := math.Ceil(seconds * float64(tickRate))
	if ticks <= 0 || ticks > float64(^uint64(0)) {
		return 0, fmt.Errorf("post-revive-protection-seconds overflows tick duration")
	}
	return uint64(ticks), nil
}

func logStepReport(report worldruntime.StepReport) {
	for _, item := range report.CommandErrors {
		log.Printf("world command error: tick=%d command=%s session=%d err=%v", report.Tick, item.Command, item.SessionID, item.Err)
	}
	for _, item := range report.ActionRejections {
		log.Printf("world action rejected: tick=%d action=%s session=%d err=%v", report.Tick, item.Action, item.SessionID, item.Err)
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
