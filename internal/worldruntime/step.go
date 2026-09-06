package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/replication"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) Step(tick uint64, delta time.Duration) StepReport {
	measure := r.config.CollectMetrics
	var totalStart time.Time
	if measure {
		totalStart = time.Now()
	}

	report := StepReport{Tick: tick}
	var stageStart time.Time
	if measure {
		stageStart = time.Now()
	}
	report.Metrics.CommandQueueDepthBefore = r.queue.depth()
	commands := r.queue.drain(r.config.MaxCommandsPerTick)
	report.Metrics.CommandsDrained = len(commands)
	for _, cmd := range commands {
		switch c := cmd.(type) {
		case registerSessionCommand:
			r.applyRegister(cmd.name(), c, &report)
		case unregisterSessionCommand:
			r.applyUnregister(cmd.name(), c, &report)
		case characterAdmissionCommand:
			err := r.characterIdentities.validateAdmission(c.identity)
			if err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), Err: err})
			}
			completeWorldOwnerCommand(c.completion, err)
		case joinCommand:
			beforeErrors := len(report.CommandErrors)
			r.applyJoin(cmd.name(), c.request, &report)
			var joinErr error
			if len(report.CommandErrors) > beforeErrors {
				joinErr = report.CommandErrors[len(report.CommandErrors)-1].Err
			}
			completeWorldOwnerCommand(c.completion, joinErr)
		case ownershipLookupCommand:
			fence, err := r.characterIdentities.currentOwnership(c.identity)
			if err == nil && c.result != nil {
				*c.result = fence
			}
			if err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), Err: err})
			}
			completeWorldOwnerCommand(c.completion, err)
		case ownershipTransferCommand:
			err := r.applyOwnershipTransfer(c.request)
			if err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.request.Expected.SessionID, Err: err})
			}
			completeWorldOwnerCommand(c.completion, err)
		case leaveCommand:
			r.applyLeave(cmd.name(), c, &report)
		case moveInputCommand:
			r.applyMove(cmd.name(), c, &report)
		case teleportCommand:
			r.applyTeleport(cmd.name(), c, &report)
		case teleportBatchCommand:
			r.applyTeleportBatch(cmd.name(), c, &report)
		case spawnEntityCommand:
			r.applySpawnEntity(cmd.name(), c.request, &report)
		case respawnCommand:
			r.applyRespawn(cmd.name(), c.request, &report)
		case setRespawnCheckpointCommand:
			r.applySetRespawnCheckpoint(cmd.name(), c, &report)
		case useActionCommand:
			r.applyUseAction(cmd.name(), c, tick, delta, &report)
		case npcCommand:
			r.applyInteractNPC(cmd.name(), c, tick, &report)
		case shopCommand:
			r.applyShopCommand(cmd.name(), c, tick, &report)
		case setBlockerCommand:
			r.applySetBlocker(cmd.name(), c, &report)
		}
	}
	// Inventory bootstrap is world-owned just like character/vitals state. Delivery uses the
	// existing bounded Reliable queue; backpressure leaves the session pending for a later tick.
	r.replicatePendingInventories(tick, &report)
	// Policy due 在 queued Client intents 之後、simulation 前執行。若同一 tick 有 move，
	// 它仍先以 Defeated 規則 consume 並清零，respawn 後不會沿用該 input。
	r.applyDueRespawns(tick, &report)
	if measure {
		report.Metrics.CommandDuration = time.Since(stageStart)
	}

	// Server-owned PvE intent is decided on the same single-owner world path as Client intent.
	// It may update movement direction or dispatch an existing Combat Action, but never writes
	// final position/damage outside the authoritative simulation/combat services.
	r.stepAutonomousMeleeAgents(tick, delta, &report)

	if measure {
		stageStart = time.Now()
	}
	report.TickErrors = r.world.Tick(float32(delta.Seconds()))
	if measure {
		report.Metrics.SimulationDuration = time.Since(stageStart)
	}

	// Siege objective truth consumes post-simulation position/defeat/team state. Reuse this
	// stable list later for SiegeMatchState replication so D.2B adds no extra session sort.
	var siegeSessions []*session.Session
	if r.siege != nil {
		siegeSessions = r.sessions.List()
		r.updateSiegeObjectives(siegeSessions, delta)
	}

	// Autosave captures the post-simulation authoritative state into the bounded process-local
	// outbox only. Journal fsync / Store CAS remain outside the world owner.
	r.autosaveCharacterStates(tick, &report)

	if measure {
		stageStart = time.Now()
	}
	r.replicateDynamicState(tick, &report)
	if measure {
		report.Metrics.DynamicReplicationDuration = time.Since(stageStart)
	}
	// Siege match state has its own Reliable delivery stamp. It is intentionally separate
	// from WorldDynamicState so phase/team truth cannot be inferred from visual blocker state.
	r.replicateSiegeState(tick, &report, siegeSessions)

	snapshotRan := false
	initialBootstrapHadLifecycle := false
	if tick%r.config.SnapshotEveryTicks == 0 {
		snapshotRan = true
		sessions := r.sessions.List()
		report.Metrics.SessionsReplicated = len(sessions)

		globalBudget := r.config.MaxLifecycleMessagesPerSnapshot
		if r.lifecycleChurnActive {
			globalBudget = r.config.MaxChurnLifecycleMessagesPerSnapshot
		}
		report.Metrics.LifecycleGlobalBudget = globalBudget

		if measure {
			stageStart = time.Now()
		}
		frame := r.replicationFrameBuilder.Build(r.world, tick)
		if measure {
			report.Metrics.ReplicationFrameBuildDuration = time.Since(stageStart)
		}

		startIndex := 0
		if len(sessions) > 0 {
			startIndex = r.lifecycleSessionCursor % len(sessions)
			if startIndex < 0 {
				startIndex = 0
			}
		}
		globalRemaining := globalBudget
		budgetExhaustedNextCursor := -1
		suppressInitialSnapshots := r.suppressInitialBootstrapSnapshots()

		for order := 0; order < len(sessions); order++ {
			index := (startIndex + order) % len(sessions)
			s := sessions[index]
			self, _, ok := frame.Entity(s.EntityID)
			if !ok {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: "replicate", SessionID: s.ID, Err: ErrSessionEntityNotFound})
				continue
			}
			if measure {
				stageStart = time.Now()
			}
			visible, queryStats := frame.Spatial.QueryRadiusInto(self.Transform.Position, s.AOIRadius, r.config.AOIOptions, r.replicationVisibleScratch)
			r.replicationVisibleScratch = visible
			if measure {
				report.Metrics.AOIDuration += time.Since(stageStart)
			}
			report.Metrics.AOIQueries++
			report.Metrics.AOICandidates += queryStats.CandidateEntities
			report.Metrics.AOIVisible += queryStats.MatchedEntities
			report.Metrics.AOISharedCandidateBuilds += queryStats.SharedCandidateBuilds
			report.Metrics.AOISharedCandidateReuses += queryStats.SharedCandidateReuses
			report.Metrics.AOIPhysicalCandidateScans += queryStats.SharedCandidateScans

			maxMessages := r.config.MaxLifecyclePerSessionBuild
			if globalBudget > 0 {
				if globalRemaining <= 0 {
					maxMessages = -1
				} else if maxMessages <= 0 || maxMessages > globalRemaining {
					maxMessages = globalRemaining
				}
			}
			connection := s.Connection()
			lifecycleLimits := replication.LifecycleLimits{
				MaxSpawns:   r.config.MaxSpawnsPerSessionBuild,
				MaxDespawns: r.config.MaxDespawnsPerSessionBuild,
				MaxMessages: maxMessages,
			}
			if suppressInitialSnapshots && !r.replication.NeedsLifecycleWork(s.ID, frame, visible) {
				lifecycleLimits.MaxMessages = -1
			}
			if measure {
				stageStart = time.Now()
			}
			var batch replication.Batch
			if _, immediate := connection.(session.ImmediateRealtimeConnection); immediate {
				batch = r.replication.BuildFrameBorrowedLifecycleFirst(s.ID, s.EntityID, s.LastProcessedInputSequence(), frame, visible, lifecycleLimits)
			} else {
				batch = r.replication.BuildFrameLifecycleFirst(s.ID, s.EntityID, s.LastProcessedInputSequence(), frame, visible, lifecycleLimits)
			}
			if measure {
				report.Metrics.ReplicationBuildDuration += time.Since(stageStart)
			}
			report.Metrics.OutboundMessages += len(batch.Messages)
			report.Metrics.SnapshotCandidates += batch.Stats.SnapshotCandidates
			report.Metrics.SnapshotTransforms += batch.Stats.SnapshotSelected
			report.Metrics.SnapshotDeferred += batch.Stats.SnapshotDeferred
			report.Metrics.SnapshotForcedRefreshes += batch.Stats.ForcedRefreshCandidates
			report.Metrics.SnapshotNearTransforms += batch.Stats.NearSelected
			report.Metrics.SnapshotMidTransforms += batch.Stats.MidSelected
			report.Metrics.SnapshotFarTransforms += batch.Stats.FarSelected
			report.Metrics.SpawnCandidates += batch.Stats.SpawnCandidates
			report.Metrics.SpawnSelected += batch.Stats.SpawnSelected
			report.Metrics.SpawnDeferred += batch.Stats.SpawnDeferred
			report.Metrics.DespawnCandidates += batch.Stats.DespawnCandidates
			report.Metrics.DespawnSelected += batch.Stats.DespawnSelected
			report.Metrics.DespawnDeferred += batch.Stats.DespawnDeferred

			if batch.Stats.SpawnCandidates > 0 || batch.Stats.SpawnSelected > 0 || batch.Stats.SpawnDeferred > 0 ||
				batch.Stats.DespawnCandidates > 0 || batch.Stats.DespawnSelected > 0 || batch.Stats.DespawnDeferred > 0 {
				initialBootstrapHadLifecycle = true
			}

			selectedLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
			report.Metrics.LifecycleGlobalSelected += selectedLifecycle

			if batch.Stats.DespawnCandidates > 0 && !r.lifecycleChurnActive {
				r.lifecycleChurnActive = true
				if r.config.MaxChurnLifecycleMessagesPerSnapshot > 0 && (globalBudget <= 0 || r.config.MaxChurnLifecycleMessagesPerSnapshot < globalBudget) {
					globalBudget = r.config.MaxChurnLifecycleMessagesPerSnapshot
					report.Metrics.LifecycleGlobalBudget = globalBudget
				}
			}
			if globalBudget > 0 {
				globalRemaining = globalBudget - report.Metrics.LifecycleGlobalSelected
				if globalRemaining < 0 {
					globalRemaining = 0
				}
				if globalRemaining == 0 && budgetExhaustedNextCursor < 0 {
					report.Metrics.LifecycleGlobalBudgetExhausted = true
					budgetExhaustedNextCursor = (index + 1) % len(sessions)
				}
			}

			if measure {
				stageStart = time.Now()
			}
			lifecycleBackpressured := false
			for _, out := range batch.Messages {
				lifecycle := isLifecycleMessage(out.Message)
				if lifecycle && lifecycleBackpressured {
					continue
				}
				envelope := protocol.Envelope{Delivery: out.Delivery, Sequence: s.NextOutboundSequence(out.Delivery), ServerTick: tick, Message: out.Message}
				if err := connection.TrySend(envelope); err != nil {
					if lifecycle && errors.Is(err, session.ErrBackpressure) {
						lifecycleBackpressured = true
						report.Metrics.LifecycleBackpressureStops++
						continue
					}
					report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: out.Delivery, MessageType: out.Message.Type(), Err: err})
					continue
				}
				confirmLifecycleDelivery(r, s.ID, out.Message)
			}
			if measure {
				report.Metrics.DeliveryDuration += time.Since(stageStart)
			}
		}

		if len(sessions) > 0 {
			if budgetExhaustedNextCursor >= 0 {
				r.lifecycleSessionCursor = budgetExhaustedNextCursor
			} else {
				r.lifecycleSessionCursor = (startIndex + 1) % len(sessions)
			}
		}
	}

	if snapshotRan {
		r.reconcileRespawnVitalsAfterSnapshot()
	}

	if measure {
		stageStart = time.Now()
	}
	r.replicateEntityVitals(tick, &report)
	if measure {
		report.Metrics.VitalsReplicationDuration = time.Since(stageStart)
	}
	if snapshotRan {
		r.observeInitialBootstrapSnapshot(report.Metrics.SessionsReplicated, initialBootstrapHadLifecycle)
	}

	report.Metrics.CommandQueueDepthAfter = r.queue.depth()
	if measure {
		report.Metrics.TotalDuration = time.Since(totalStart)
	}
	return report
}

func isLifecycleMessage(message protocol.Message) bool {
	switch message.(type) {
	case protocol.EntitySpawn, protocol.EntityDespawn:
		return true
	default:
		return false
	}
}
