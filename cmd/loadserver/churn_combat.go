package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	loadlabCombatActionsPerTick = 16
	loadlabCombatBatchInterval  = 50 * time.Millisecond
	loadlabStepFenceTimeout     = 5 * time.Second
)

var (
	churnCombatPairsPerGroup          *int
	s3e9MixedGameplayDuration         *time.Duration
	s3e9MixedGameplayWaveInterval     *time.Duration
	s3e9MixedGameplayDynamicInterval  *time.Duration
	s3e9MixedGameplayActionID         *string
	s3e9MixedGameplayDynamicBlockerID string
)

// bindChurnCombatFlags keeps load-test-only CLI registration explicit from main.
// This avoids package-init registration becoming an invisible dependency of the
// executable contract and makes the flags directly testable with an isolated FlagSet.
func bindChurnCombatFlags(fs *flag.FlagSet) {
	churnCombatPairsPerGroup = fs.Int(
		"churn-combat-pairs-per-group",
		0,
		"Deterministic basic-attack pairs per teleport cluster in each churn round; 0 disables combat overlap",
	)
	s3e9MixedGameplayDuration = fs.Duration(
		"s3e9-mixed-gameplay-duration",
		0,
		"S3-E.9 sustained mixed gameplay injection duration during teleport-churn round 1; 0 disables",
	)
	s3e9MixedGameplayWaveInterval = fs.Duration(
		"s3e9-mixed-gameplay-wave-interval",
		150*time.Millisecond,
		"S3-E.9 sustained hot-entity action wave interval",
	)
	s3e9MixedGameplayDynamicInterval = fs.Duration(
		"s3e9-mixed-gameplay-dynamic-interval",
		1500*time.Millisecond,
		"S3-E.9 generic dynamic-world blocker toggle interval",
	)
	s3e9MixedGameplayActionID = fs.String(
		"s3e9-mixed-gameplay-action",
		"soak-attack",
		"S3-E.9 low-damage load-lab combat action ID",
	)
}

// configureS3E9DynamicStress selects an existing enabled blocker only for the
// load-test process. It does not promote that blocker to a gameplay objective or
// persist any new world semantics; it merely drives the existing DynamicWorld path.
func configureS3E9DynamicStress(def gameplayworld.Definition) error {
	s3e9MixedGameplayDynamicBlockerID = ""
	if s3e9MixedGameplayDuration == nil || *s3e9MixedGameplayDuration <= 0 {
		return nil
	}
	for _, blocker := range def.Blockers {
		if blocker.Enabled {
			s3e9MixedGameplayDynamicBlockerID = blocker.ID
			return nil
		}
	}
	return fmt.Errorf("loadlab: S3-E.9 mixed gameplay requires an enabled blocker for dynamic-world stress")
}

type churnCombatMetrics struct {
	ActionsApplied                uint64 `json:"combat_actions_applied"`
	ActionRejections              uint64 `json:"action_rejections"`
	DirtyVitalsSelected           uint64 `json:"dirty_vitals_selected"`
	MaxDirtyVitalsSelectedPerTick int    `json:"max_dirty_vitals_selected_per_tick"`
}

func enqueueChurnCombatActions(runtime *worldruntime.Runtime, round int, pairs []loadlab.EntityCombatPair) error {
	if err := enqueueCombatPairsPaced(runtime, uint32(round), "basic-attack", pairs, loadlabCombatActionsPerTick, loadlabCombatBatchInterval); err != nil {
		return fmt.Errorf("enqueue churn combat round=%d: %w", round, err)
	}

	if round == 1 && s3e9MixedGameplayDuration != nil && *s3e9MixedGameplayDuration > 0 {
		if err := validateS3E9MixedGameplayConfig(); err != nil {
			return err
		}
		hotPairs := make([]loadlab.EntityCombatPair, 0, len(pairs)/2)
		for _, pair := range pairs {
			if loadlab.S3E9MixedStationaryEntity(pair.ActorID) && loadlab.S3E9MixedStationaryEntity(pair.TargetID) {
				hotPairs = append(hotPairs, pair)
			}
		}
		if len(hotPairs) == 0 {
			return fmt.Errorf("loadlab: S3-E.9 mixed gameplay has no stationary hot combat pairs")
		}
		waves := int(*s3e9MixedGameplayDuration / *s3e9MixedGameplayWaveInterval)
		log.Printf("S3-E.9 sustained mixed gameplay starting: hot_pairs=%d waves=%d wave_interval=%s duration=%s dynamic_interval=%s dynamic_blocker=%s action=%s", len(hotPairs), waves, s3e9MixedGameplayWaveInterval.String(), s3e9MixedGameplayDuration.String(), s3e9MixedGameplayDynamicInterval.String(), s3e9MixedGameplayDynamicBlockerID, *s3e9MixedGameplayActionID)
		go runS3E9MixedGameplay(runtime, round, hotPairs, waves)
	}
	return nil
}

func enqueueCombatPairsPaced(runtime *worldruntime.Runtime, sequence uint32, actionID string, pairs []loadlab.EntityCombatPair, batchSize int, batchInterval time.Duration) error {
	if batchSize <= 0 {
		return fmt.Errorf("loadlab: combat batch size must be > 0")
	}
	for start := 0; start < len(pairs); start += batchSize {
		end := start + batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		for _, pair := range pairs[start:end] {
			action := protocol.ClientUseAction{
				ActionID:   actionID,
				TargetKind: protocol.ActionTargetEntity,
				TargetID:   strconv.FormatUint(uint64(pair.TargetID), 10),
			}
			// Load Lab 的 tcpudp Server 以同一 atomic order 配發 SessionID / EntityID，
			// 因此 deterministic player IDs 可直接對應 SessionID。所有 mutation 仍
			// 透過 Runtime command queue，分批只避免壓測器人工製造單 tick burst。
			if err := runtime.EnqueueUseAction(session.ID(pair.ActorID), sequence, action); err != nil {
				return fmt.Errorf("actor=%d target=%d sequence=%d: %w", pair.ActorID, pair.TargetID, sequence, err)
			}
		}
		if end < len(pairs) && batchInterval > 0 {
			time.Sleep(batchInterval)
		}
	}
	return nil
}

func validateS3E9MixedGameplayConfig() error {
	if s3e9MixedGameplayDuration == nil || s3e9MixedGameplayWaveInterval == nil || s3e9MixedGameplayDynamicInterval == nil || s3e9MixedGameplayActionID == nil {
		return fmt.Errorf("loadlab: S3-E.9 mixed gameplay flags are not bound")
	}
	if *s3e9MixedGameplayWaveInterval <= 0 || *s3e9MixedGameplayDynamicInterval <= 0 {
		return fmt.Errorf("loadlab: S3-E.9 wave/dynamic intervals must be > 0")
	}
	if *s3e9MixedGameplayDuration < *s3e9MixedGameplayWaveInterval || *s3e9MixedGameplayDuration%*s3e9MixedGameplayWaveInterval != 0 {
		return fmt.Errorf("loadlab: S3-E.9 duration must be a positive multiple of wave interval")
	}
	if *s3e9MixedGameplayDynamicInterval%*s3e9MixedGameplayWaveInterval != 0 {
		return fmt.Errorf("loadlab: S3-E.9 dynamic interval must be a multiple of wave interval")
	}
	if *s3e9MixedGameplayWaveInterval < loadlabCombatBatchInterval {
		return fmt.Errorf("loadlab: S3-E.9 wave interval must be >= combat batch interval")
	}
	if *s3e9MixedGameplayActionID == "" {
		return fmt.Errorf("loadlab: S3-E.9 action ID is required")
	}
	if s3e9MixedGameplayDynamicBlockerID == "" {
		return fmt.Errorf("loadlab: S3-E.9 dynamic blocker is required")
	}
	return nil
}

func waitForS3E9StepBoundary(runtime *worldruntime.Runtime) error {
	reached, err := runtime.EnqueueStepFence()
	if err != nil {
		return fmt.Errorf("enqueue authoritative step fence: %w", err)
	}
	timer := time.NewTimer(loadlabStepFenceTimeout)
	defer timer.Stop()
	select {
	case <-reached:
		return nil
	case <-timer.C:
		return fmt.Errorf("authoritative step fence timed out after %s", loadlabStepFenceTimeout)
	}
}

func runS3E9MixedGameplay(runtime *worldruntime.Runtime, round int, hotPairs []loadlab.EntityCombatPair, waves int) {
	dynamicEvery := int(*s3e9MixedGameplayDynamicInterval / *s3e9MixedGameplayWaveInterval)
	batchesPerWave := int(*s3e9MixedGameplayWaveInterval / loadlabCombatBatchInterval)
	if batchesPerWave < 1 {
		batchesPerWave = 1
	}
	batchSize := (len(hotPairs) + batchesPerWave - 1) / batchesPerWave
	blockerEnabled := true
	for wave := 1; wave <= waves; wave++ {
		waveStarted := time.Now()
		sequence := uint32(round + wave)
		if err := enqueueCombatPairsPaced(runtime, sequence, *s3e9MixedGameplayActionID, hotPairs, batchSize, loadlabCombatBatchInterval); err != nil {
			log.Printf("S3-E.9 sustained action enqueue failed: wave=%d err=%v", wave, err)
			return
		}
		if wave%dynamicEvery == 0 {
			blockerEnabled = !blockerEnabled
			if err := runtime.EnqueueSetBlocker(s3e9MixedGameplayDynamicBlockerID, blockerEnabled); err != nil {
				log.Printf("S3-E.9 dynamic-world enqueue failed: wave=%d blocker=%s enabled=%t err=%v", wave, s3e9MixedGameplayDynamicBlockerID, blockerEnabled, err)
				return
			}
		}
		// Wall-clock pacing alone can collapse two waves into one authoritative tick when a
		// hosted runner stalls. The queue fence preserves the intended wave interval when healthy,
		// but guarantees the next wave cannot be drained by the same world Step when it is not.
		if err := waitForS3E9StepBoundary(runtime); err != nil {
			log.Printf("S3-E.9 authoritative step fence failed: wave=%d err=%v", wave, err)
			return
		}
		if remaining := *s3e9MixedGameplayWaveInterval - time.Since(waveStarted); remaining > 0 {
			time.Sleep(remaining)
		}
	}
	if !blockerEnabled {
		if err := runtime.EnqueueSetBlocker(s3e9MixedGameplayDynamicBlockerID, true); err != nil {
			log.Printf("S3-E.9 final dynamic-world restore enqueue failed: blocker=%s err=%v", s3e9MixedGameplayDynamicBlockerID, err)
		}
	}
	log.Printf("S3-E.9 sustained mixed gameplay injection completed: hot_pairs=%d waves=%d dynamic_blocker=%s", len(hotPairs), waves, s3e9MixedGameplayDynamicBlockerID)
}
