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
		case joinCommand:
			r.applyJoin(cmd.name(), c.request, &report)
		case leaveCommand:
			r.applyLeave(cmd.name(), c.id, &report)
		case moveInputCommand:
			r.applyMove(cmd.name(), c, &report)
		case teleportCommand:
			r.applyTeleport(cmd.name(), c, &report)
		case teleportBatchCommand:
			r.applyTeleportBatch(cmd.name(), c, &report)
		case useActionCommand:
			r.applyUseAction(cmd.name(), c, tick, delta, &report)
		case setBlockerCommand:
			r.applySetBlocker(cmd.name(), c, &report)
		}
	}
	if measure {
		report.Metrics.CommandDuration = time.Since(stageStart)
	}

	if measure {
		stageStart = time.Now()
	}
	report.TickErrors = r.world.Tick(float32(delta.Seconds()))
	if measure {
		report.Metrics.SimulationDuration = time.Since(stageStart)
	}

	if measure {
		stageStart = time.Now()
	}
	r.replicateDynamicState(tick, &report)
	if measure {
		report.Metrics.DynamicReplicationDuration = time.Since(stageStart)
	}

	if tick%r.config.SnapshotEveryTicks == 0 {
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

			selectedLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
			report.Metrics.LifecycleGlobalSelected += selectedLifecycle

			// 第一個 departed candidate 就代表這不是 pure bootstrap，而是 mixed AOI churn。
			// 立即把本 snapshot 的 global ceiling 收斂到較低 churn budget；第一個 Session 最多只先用32筆。
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

	if measure {
		stageStart = time.Now()
	}
	r.replicateEntityVitals(tick, &report)
	if measure {
		report.Metrics.VitalsReplicationDuration = time.Since(stageStart)
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

func confirmLifecycleDelivery(r *Runtime, sessionID session.ID, message protocol.Message) {
	switch value := message.(type) {
	case protocol.EntitySpawn:
		r.replication.ConfirmSpawn(sessionID, value.EntityID)
		r.queueEntityVitalsForSession(sessionID, value.EntityID)
	case protocol.EntityDespawn:
		r.replication.ConfirmDespawn(sessionID, value.EntityID)
		r.confirmEntityDespawnVitals(sessionID, value.EntityID)
	}
}
