package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var churnCombatPairsPerGroup = flag.Int(
	"churn-combat-pairs-per-group",
	0,
	"Deterministic basic-attack pairs per teleport cluster in each churn round; 0 disables combat overlap",
)

var (
	s3e9MixedGameplayDuration = flag.Duration(
		"s3e9-mixed-gameplay-duration",
		0,
		"S3-E.9 sustained mixed gameplay injection duration during teleport-churn round 1; 0 disables",
	)
	s3e9MixedGameplayWaveInterval = flag.Duration(
		"s3e9-mixed-gameplay-wave-interval",
		100*time.Millisecond,
		"S3-E.9 sustained hot-entity action wave interval",
	)
	s3e9MixedGameplayObjectiveInterval = flag.Duration(
		"s3e9-mixed-gameplay-objective-interval",
		time.Second,
		"S3-E.9 main-gate dynamic/objective toggle interval",
	)
	s3e9MixedGameplayActionID = flag.String(
		"s3e9-mixed-gameplay-action",
		"soak-attack",
		"S3-E.9 low-damage load-lab combat action ID",
	)
)

type churnCombatMetrics struct {
	ActionsApplied                uint64 `json:"combat_actions_applied"`
	ActionRejections              uint64 `json:"action_rejections"`
	DirtyVitalsSelected           uint64 `json:"dirty_vitals_selected"`
	MaxDirtyVitalsSelectedPerTick int    `json:"max_dirty_vitals_selected_per_tick"`
}

func enqueueChurnCombatActions(runtime *worldruntime.Runtime, round int, pairs []loadlab.EntityCombatPair) error {
	for _, pair := range pairs {
		action := protocol.ClientUseAction{
			ActionID:   "basic-attack",
			TargetKind: protocol.ActionTargetEntity,
			TargetID:   strconv.FormatUint(uint64(pair.TargetID), 10),
		}
		// Load Lab 的 tcpudp Server 以同一 atomic order 配發 SessionID / EntityID，
		// 因此 deterministic player IDs 可直接對應 SessionID；每輪 round number 也是
		// 每個 actor 嚴格遞增的 action sequence。
		if err := runtime.EnqueueUseAction(session.ID(pair.ActorID), uint32(round), action); err != nil {
			return fmt.Errorf("enqueue churn combat actor=%d target=%d round=%d: %w", pair.ActorID, pair.TargetID, round, err)
		}
	}

	if round == 1 && *s3e9MixedGameplayDuration > 0 {
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
		log.Printf("S3-E.9 sustained mixed gameplay starting: hot_pairs=%d waves=%d wave_interval=%s duration=%s objective_interval=%s action=%s", len(hotPairs), waves, s3e9MixedGameplayWaveInterval.String(), s3e9MixedGameplayDuration.String(), s3e9MixedGameplayObjectiveInterval.String(), *s3e9MixedGameplayActionID)
		go runS3E9MixedGameplay(runtime, round, hotPairs, waves)
	}
	return nil
}

func validateS3E9MixedGameplayConfig() error {
	if *s3e9MixedGameplayWaveInterval <= 0 || *s3e9MixedGameplayObjectiveInterval <= 0 {
		return fmt.Errorf("loadlab: S3-E.9 wave/objective intervals must be > 0")
	}
	if *s3e9MixedGameplayDuration < *s3e9MixedGameplayWaveInterval || *s3e9MixedGameplayDuration%*s3e9MixedGameplayWaveInterval != 0 {
		return fmt.Errorf("loadlab: S3-E.9 duration must be a positive multiple of wave interval")
	}
	if *s3e9MixedGameplayObjectiveInterval%*s3e9MixedGameplayWaveInterval != 0 {
		return fmt.Errorf("loadlab: S3-E.9 objective interval must be a multiple of wave interval")
	}
	if *s3e9MixedGameplayActionID == "" {
		return fmt.Errorf("loadlab: S3-E.9 action ID is required")
	}
	return nil
}

func runS3E9MixedGameplay(runtime *worldruntime.Runtime, round int, hotPairs []loadlab.EntityCombatPair, waves int) {
	ticker := time.NewTicker(*s3e9MixedGameplayWaveInterval)
	defer ticker.Stop()
	objectiveEvery := int(*s3e9MixedGameplayObjectiveInterval / *s3e9MixedGameplayWaveInterval)
	blockerEnabled := true
	for wave := 1; wave <= waves; wave++ {
		<-ticker.C
		sequence := uint32(round + wave)
		for _, pair := range hotPairs {
			action := protocol.ClientUseAction{
				ActionID:   *s3e9MixedGameplayActionID,
				TargetKind: protocol.ActionTargetEntity,
				TargetID:   strconv.FormatUint(uint64(pair.TargetID), 10),
			}
			if err := runtime.EnqueueUseAction(session.ID(pair.ActorID), sequence, action); err != nil {
				log.Printf("S3-E.9 sustained action enqueue failed: wave=%d actor=%d target=%d err=%v", wave, pair.ActorID, pair.TargetID, err)
			}
		}
		if wave%objectiveEvery == 0 {
			blockerEnabled = !blockerEnabled
			if err := runtime.EnqueueSetBlocker("main-gate", blockerEnabled); err != nil {
				log.Printf("S3-E.9 objective enqueue failed: wave=%d enabled=%t err=%v", wave, blockerEnabled, err)
			}
		}
	}
	if !blockerEnabled {
		if err := runtime.EnqueueSetBlocker("main-gate", true); err != nil {
			log.Printf("S3-E.9 final objective restore enqueue failed: err=%v", err)
		}
	}
	log.Printf("S3-E.9 sustained mixed gameplay injection completed: hot_pairs=%d waves=%d", len(hotPairs), waves)
}
