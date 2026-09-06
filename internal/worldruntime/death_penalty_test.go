package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/deathpenalty"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDeathPenaltyPvEForfeitsCheckpointAfterCurrentRespawnBindingExactlyOnce(t *testing.T) {
	rt, respawn := makeRespawnContextRuntime(t, world.EntityMonster)
	penalty := makeDeathPenaltyService(t)
	rt.deathPenalty = penalty
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueAcquireRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("checkpoint=%#v", report.CommandErrors)
	}
	if checkpoint, ok := respawn.Checkpoint(2); !ok || checkpoint != "checkpoint" {
		t.Fatalf("checkpoint=%q ok=%v", checkpoint, ok)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(3, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", defeat)
	}
	if defeat.Metrics.DeathOutcomesRecorded != 1 || defeat.Metrics.DeathPenaltyTransactionsApplied != 1 || defeat.Metrics.DeathPenaltyCheckpointForfeits != 1 || defeat.Metrics.RespawnsScheduled != 1 {
		t.Fatalf("defeat metrics=%#v", defeat.Metrics)
	}
	if revision := rt.deathRevision[2]; revision != 1 {
		t.Fatalf("death revision=%d", revision)
	}
	if revision, ok := penalty.AppliedRevision(2); !ok || revision != 1 {
		t.Fatalf("penalty revision=%d ok=%v", revision, ok)
	}

	// Respawn schedule 必須先於 penalty綁定；checkpoint雖已被消耗，本次 death outcome仍回原 checkpoint。
	pending, ok := respawn.Pending(2)
	if !ok || pending.Context != respawnpolicy.DeathContextPvE || pending.SpawnPointID != "checkpoint" || pending.DueTick != 5 {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}
	if _, ok := respawn.Checkpoint(2); ok {
		t.Fatal("checkpoint was not forfeited after death binding")
	}

	// Restart intent 可以提前送達，但仍不得越過 policy due tick。
	if err := rt.EnqueueRespawnRequest(2, 1, protocol.ClientRespawnRequest{}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 || report.Metrics.RespawnsApplied != 0 {
		t.Fatalf("before due=%#v", report)
	}
	due := rt.Step(5, 50*time.Millisecond)
	if len(due.CommandErrors) != 0 || due.Metrics.RespawnsApplied != 1 {
		t.Fatalf("due=%#v", due)
	}
	entity, ok := rt.world.Entity(2)
	if !ok || entity.Transform.Position != (world.Position{X: 100, Layer: 0}) {
		t.Fatalf("bound checkpoint respawn position=%#v ok=%v", entity.Transform.Position, ok)
	}

	// 復活後重新取得 checkpoint；重放舊 revision 必須是 exactly-once no-op，不能再次清掉新取得的 checkpoint。
	if err := rt.EnqueueAcquireRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(6, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("reacquire=%#v", report.CommandErrors)
	}
	duplicateReport := StepReport{Tick: 6}
	rt.applyDeathPenalty(deathOutcome{EntityID: 2, Revision: 1, Context: respawnpolicy.DeathContextPvE, DefeatedTick: 3}, &duplicateReport)
	if len(duplicateReport.CommandErrors) != 0 || duplicateReport.Metrics.DeathPenaltyTransactionsApplied != 0 || duplicateReport.Metrics.DeathPenaltyCheckpointForfeits != 0 {
		t.Fatalf("duplicate penalty=%#v", duplicateReport)
	}
	if checkpoint, ok := respawn.Checkpoint(2); !ok || checkpoint != "checkpoint" {
		t.Fatalf("duplicate revision consumed reacquired checkpoint=%q ok=%v", checkpoint, ok)
	}

	// 新的 authoritative defeat才產生 revision=2，並可再次 exactly-once消耗 checkpoint。
	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	second := rt.Step(13, 50*time.Millisecond)
	if len(second.CommandErrors) != 0 || len(second.ActionRejections) != 0 {
		t.Fatalf("second defeat=%#v", second)
	}
	if rt.deathRevision[2] != 2 || second.Metrics.DeathOutcomesRecorded != 1 || second.Metrics.DeathPenaltyTransactionsApplied != 1 || second.Metrics.DeathPenaltyCheckpointForfeits != 1 {
		t.Fatalf("second defeat revision=%d metrics=%#v", rt.deathRevision[2], second.Metrics)
	}
	if _, ok := respawn.Checkpoint(2); ok {
		t.Fatal("second defeat did not forfeit reacquired checkpoint")
	}
}

func TestDeathPenaltyContextPolicyAndLeaveCleanup(t *testing.T) {
	rt, respawn := makeRespawnContextRuntime(t, world.EntityPlayer)
	penalty := makeDeathPenaltyService(t)
	rt.deathPenalty = penalty
	rt.Step(1, 50*time.Millisecond)

	if err := rt.EnqueueAcquireRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("checkpoint=%#v", report.CommandErrors)
	}
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(3, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("pvp defeat=%#v", defeat)
	}
	if defeat.Metrics.DeathOutcomesRecorded != 1 || defeat.Metrics.DeathPenaltyTransactionsApplied != 1 || defeat.Metrics.DeathPenaltyCheckpointForfeits != 0 {
		t.Fatalf("pvp metrics=%#v", defeat.Metrics)
	}
	pending, ok := respawn.Pending(2)
	if !ok || pending.Context != respawnpolicy.DeathContextPvP || pending.SpawnPointID != "safe" {
		t.Fatalf("pvp pending=%#v ok=%v", pending, ok)
	}
	if checkpoint, ok := respawn.Checkpoint(2); !ok || checkpoint != "checkpoint" {
		t.Fatalf("pvp death incorrectly forfeited checkpoint=%q ok=%v", checkpoint, ok)
	}
	if revision, ok := penalty.AppliedRevision(2); !ok || revision != 1 {
		t.Fatalf("pvp penalty revision=%d ok=%v", revision, ok)
	}

	if err := rt.EnqueueLeave(2); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave=%#v", report.CommandErrors)
	}
	if _, ok := rt.deathRevision[2]; ok {
		t.Fatal("death revision survived leave")
	}
	if _, ok := penalty.AppliedRevision(2); ok {
		t.Fatal("death penalty applied revision survived leave")
	}
}

func makeDeathPenaltyService(t *testing.T) *deathpenalty.Service {
	t.Helper()
	service, err := deathpenalty.NewService(deathpenalty.Definition{
		SchemaVersion:             deathpenalty.SchemaVersion,
		Revision:                  "runtime-test-s3f7",
		CheckpointForfeitContexts: []respawnpolicy.DeathContext{respawnpolicy.DeathContextPvE},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
