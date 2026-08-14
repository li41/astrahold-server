package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDeathOutcomeEventCapturesBoundRespawnAndAppliedPenalty(t *testing.T) {
	rt, respawn := makeRespawnContextRuntime(t, world.EntityMonster)
	rt.deathPenalty = makeDeathPenaltyService(t)
	outbox, err := deathoutcome.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt.deathOutbox = outbox
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

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
		t.Fatalf("defeat=%#v", defeat)
	}
	if defeat.Metrics.DeathOutcomeEventsEnqueued != 1 || defeat.Metrics.DeathOutcomeEventEnqueueFailures != 0 {
		t.Fatalf("event metrics=%#v", defeat.Metrics)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	event := pending[0]
	if event.EventID != 1 || event.EntityID != 2 || event.DefeatRevision != 1 || event.Context != respawnpolicy.DeathContextPvE || event.DefeatedTick != 3 {
		t.Fatalf("event identity=%#v", event)
	}
	if event.RespawnPolicyRevision != "runtime-test-s3f4" || event.DeathPenaltyPolicyRevision != "runtime-test-s3f7" {
		t.Fatalf("policy revisions=%#v", event)
	}
	if !event.Respawn.Scheduled || event.Respawn.SpawnPointID != "checkpoint" || event.Respawn.SpawnClass != respawnpolicy.SpawnClassCheckpoint || event.Respawn.DueTick != 5 || event.Respawn.Position != (world.Position{X: 100, Layer: 0}) {
		t.Fatalf("respawn binding=%#v", event.Respawn)
	}
	if !event.PenaltyTransactionApplied || !event.CheckpointForfeited {
		t.Fatalf("penalty result=%#v", event)
	}
	if _, ok := respawn.Checkpoint(2); ok {
		t.Fatal("checkpoint should already be forfeited after event capture")
	}
	if err := outbox.Confirm(event.EventID); err != nil || outbox.Depth() != 0 {
		t.Fatalf("confirm err=%v depth=%d", err, outbox.Depth())
	}
}

func TestDeathOutcomeOutboxFullDoesNotRollbackGameplayTruth(t *testing.T) {
	rt, respawn := makeRespawnContextRuntime(t, world.EntityMonster)
	rt.deathPenalty = makeDeathPenaltyService(t)
	outbox, _ := deathoutcome.NewOutbox(1)
	rt.deathOutbox = outbox
	rt.Step(1, 50*time.Millisecond)

	if err := rt.EnqueueAcquireRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	rt.Step(2, 50*time.Millisecond)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	first := rt.Step(3, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || outbox.Depth() != 1 {
		t.Fatalf("first=%#v depth=%d", first, outbox.Depth())
	}
	// 保留第一個 event未 Confirm，讓 outbox持續滿載；角色仍照既有 policy復活。
	rt.Step(4, 50*time.Millisecond)
	if due := rt.Step(5, 50*time.Millisecond); len(due.CommandErrors) != 0 || due.Metrics.RespawnsApplied != 1 {
		t.Fatalf("due=%#v", due)
	}
	if err := rt.EnqueueAcquireRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(6, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("reacquire=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	second := rt.Step(13, 50*time.Millisecond)
	if len(second.CommandErrors) != 1 || !errors.Is(second.CommandErrors[0].Err, deathoutcome.ErrOutboxFull) {
		t.Fatalf("second errors=%#v", second.CommandErrors)
	}
	if second.Metrics.DeathOutcomesRecorded != 1 || second.Metrics.RespawnsScheduled != 1 || second.Metrics.DeathPenaltyTransactionsApplied != 1 || second.Metrics.DeathPenaltyCheckpointForfeits != 1 || second.Metrics.DeathOutcomeEventsEnqueued != 0 || second.Metrics.DeathOutcomeEventEnqueueFailures != 1 {
		t.Fatalf("second metrics=%#v", second.Metrics)
	}
	state, ok := rt.characters.State(2)
	if !ok || !state.Defeated || state.HP != 0 || rt.deathRevision[2] != 2 {
		t.Fatalf("character=%#v ok=%v revision=%d", state, ok, rt.deathRevision[2])
	}
	pending, ok := respawn.Pending(2)
	if !ok || pending.Context != respawnpolicy.DeathContextPvE || pending.SpawnPointID != "checkpoint" {
		t.Fatalf("respawn pending=%#v ok=%v", pending, ok)
	}
	if _, ok := respawn.Checkpoint(2); ok {
		t.Fatal("outbox failure rolled back checkpoint forfeiture")
	}
	if outbox.Depth() != 1 || outbox.Pending(1)[0].DefeatRevision != 1 {
		t.Fatalf("outbox truth=%#v", outbox.Pending(0))
	}
}
