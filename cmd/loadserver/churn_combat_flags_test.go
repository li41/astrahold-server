package main

import (
	"flag"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

func TestBindChurnCombatFlags(t *testing.T) {
	oldPairs := churnCombatPairsPerGroup
	oldDuration := s3e9MixedGameplayDuration
	oldWave := s3e9MixedGameplayWaveInterval
	oldDynamic := s3e9MixedGameplayDynamicInterval
	oldAction := s3e9MixedGameplayActionID
	oldBlocker := s3e9MixedGameplayDynamicBlockerID
	t.Cleanup(func() {
		churnCombatPairsPerGroup = oldPairs
		s3e9MixedGameplayDuration = oldDuration
		s3e9MixedGameplayWaveInterval = oldWave
		s3e9MixedGameplayDynamicInterval = oldDynamic
		s3e9MixedGameplayActionID = oldAction
		s3e9MixedGameplayDynamicBlockerID = oldBlocker
	})

	fs := flag.NewFlagSet("loadserver-test", flag.ContinueOnError)
	bindChurnCombatFlags(fs)
	if err := fs.Parse([]string{
		"-churn-combat-pairs-per-group", "56",
		"-s3e9-mixed-gameplay-duration", "42s",
		"-s3e9-mixed-gameplay-wave-interval", "150ms",
		"-s3e9-mixed-gameplay-dynamic-interval", "1500ms",
		"-s3e9-mixed-gameplay-action", "soak-attack",
	}); err != nil {
		t.Fatal(err)
	}

	if *churnCombatPairsPerGroup != 56 {
		t.Fatalf("churn combat pairs=%d, want 56", *churnCombatPairsPerGroup)
	}
	if *s3e9MixedGameplayDuration != 42*time.Second || *s3e9MixedGameplayWaveInterval != 150*time.Millisecond || *s3e9MixedGameplayDynamicInterval != 1500*time.Millisecond {
		t.Fatalf("unexpected mixed timing: duration=%s wave=%s dynamic=%s", *s3e9MixedGameplayDuration, *s3e9MixedGameplayWaveInterval, *s3e9MixedGameplayDynamicInterval)
	}
	if *s3e9MixedGameplayActionID != "soak-attack" {
		t.Fatalf("action=%q, want soak-attack", *s3e9MixedGameplayActionID)
	}
}

func TestConfigureS3E9DynamicStressUsesExistingEnabledBlocker(t *testing.T) {
	oldDuration := s3e9MixedGameplayDuration
	oldBlocker := s3e9MixedGameplayDynamicBlockerID
	t.Cleanup(func() {
		s3e9MixedGameplayDuration = oldDuration
		s3e9MixedGameplayDynamicBlockerID = oldBlocker
	})

	duration := time.Second
	s3e9MixedGameplayDuration = &duration
	def := gameplayworld.Definition{Blockers: []gameplayworld.Blocker{
		{ID: "disabled", Enabled: false},
		{ID: "current-world-blocker", Enabled: true},
	}}
	if err := configureS3E9DynamicStress(def); err != nil {
		t.Fatal(err)
	}
	if s3e9MixedGameplayDynamicBlockerID != "current-world-blocker" {
		t.Fatalf("dynamic blocker=%q, want current-world-blocker", s3e9MixedGameplayDynamicBlockerID)
	}
}

func TestConfigureS3E9DynamicStressRejectsMissingEnabledBlocker(t *testing.T) {
	oldDuration := s3e9MixedGameplayDuration
	oldBlocker := s3e9MixedGameplayDynamicBlockerID
	t.Cleanup(func() {
		s3e9MixedGameplayDuration = oldDuration
		s3e9MixedGameplayDynamicBlockerID = oldBlocker
	})

	duration := time.Second
	s3e9MixedGameplayDuration = &duration
	if err := configureS3E9DynamicStress(gameplayworld.Definition{}); err == nil {
		t.Fatal("expected missing dynamic blocker error")
	}
}
