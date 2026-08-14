package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestResurrectionOnDueTickWinsBeforePolicyRespawnAndStaysInPlace(t *testing.T) {
	rt, policy := makeResurrectionRuntime(t)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

	// 先送 move 再送 lethal action；同一 owner command phase 中 defeat 必須把 persistent input歸零。
	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(2, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 || defeat.Metrics.RespawnsScheduled != 1 {
		t.Fatalf("defeat=%#v", defeat)
	}
	pending, ok := policy.Pending(2)
	if !ok || pending.Context != respawnpolicy.DeathContextPvP || pending.DueTick != 7 {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}
	before, ok := rt.world.Entity(2)
	if !ok {
		t.Fatal("target missing")
	}
	if before.Transform.Position.X != 2 {
		t.Fatalf("defeated target moved despite lethal input clear: %#v", before.Transform.Position)
	}

	// queued actions在 applyDueRespawns 前執行，因此 exact due tick resurrection可先取消 pending。
	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	resurrected := rt.Step(7, 50*time.Millisecond)
	if len(resurrected.CommandErrors) != 0 || len(resurrected.ActionRejections) != 0 {
		t.Fatalf("resurrection=%#v", resurrected)
	}
	if resurrected.Metrics.EntityActionsApplied != 1 || resurrected.Metrics.RespawnsApplied != 0 || resurrected.Metrics.RespawnPolicyDue != 0 {
		t.Fatalf("resurrection metrics=%#v", resurrected.Metrics)
	}
	state, ok := rt.characters.State(2)
	if !ok || state.Defeated || state.HP != 60 || state.MaxHP != 200 {
		t.Fatalf("resurrected state=%#v ok=%v", state, ok)
	}
	if _, ok := policy.Pending(2); ok {
		t.Fatal("pending auto-respawn survived successful resurrection")
	}
	after, ok := rt.world.Entity(2)
	if !ok || after.Transform.Position != before.Transform.Position {
		t.Fatalf("resurrection moved target before=%#v after=%#v ok=%v", before.Transform.Position, after.Transform.Position, ok)
	}
	s2, ok := rt.sessions.Get(2)
	if !ok || s2.LastProcessedInputSequence() != 1 {
		t.Fatalf("target input history=%#v ok=%v", s2, ok)
	}

	// 沒有新的 move input時，復活後下一 tick仍不得沿用倒地前方向。
	if report := rt.Step(8, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("post-resurrection idle=%#v", report.CommandErrors)
	}
	idle, _ := rt.world.Entity(2)
	if idle.Transform.Position != before.Transform.Position {
		t.Fatalf("old movement resumed after resurrection: %#v", idle.Transform.Position)
	}
	if err := rt.EnqueueMove(2, 2, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	fresh := rt.Step(9, 50*time.Millisecond)
	if len(fresh.CommandErrors) != 0 {
		t.Fatalf("fresh move=%#v", fresh.CommandErrors)
	}
	moved, _ := rt.world.Entity(2)
	if moved.Transform.Position.X <= before.Transform.Position.X {
		t.Fatalf("fresh post-resurrection move did not advance: %#v", moved.Transform.Position)
	}
}

func TestResurrectionSequenceAndCooldownCommitOnlyOnSuccess(t *testing.T) {
	rt, policy := makeResurrectionRuntime(t)
	rt.Step(1, 50*time.Millisecond)

	// Alive target是 gameplay rejection：intent sequence消耗，但 resurrection cooldown不能被吃掉。
	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	alive := rt.Step(2, 50*time.Millisecond)
	if len(alive.ActionRejections) != 1 || !errors.Is(alive.ActionRejections[0].Err, character.ErrCharacterNotDefeated) {
		t.Fatalf("alive target rejection=%#v", alive.ActionRejections)
	}
	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	staleRejected := rt.Step(3, 50*time.Millisecond)
	if len(staleRejected.CommandErrors) != 1 || !errors.Is(staleRejected.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("rejected action sequence replay=%#v", staleRejected.CommandErrors)
	}

	// 下一個 sequence在同一 tick先看到 lethal transition，證明前一個 gameplay rejection沒有 Commit cooldown。
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(3, 2, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	success := rt.Step(4, 50*time.Millisecond)
	if len(success.CommandErrors) != 0 || len(success.ActionRejections) != 0 || success.Metrics.EntityActionsApplied != 2 {
		t.Fatalf("kill+resurrect=%#v", success)
	}
	state, _ := rt.characters.State(2)
	if state.Defeated || state.HP != 60 {
		t.Fatalf("successful resurrection state=%#v", state)
	}
	if _, ok := policy.Pending(2); ok {
		t.Fatal("successful resurrection did not cancel pending")
	}
	if err := rt.EnqueueUseAction(3, 2, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	staleSuccess := rt.Step(5, 50*time.Millisecond)
	if len(staleSuccess.CommandErrors) != 1 || !errors.Is(staleSuccess.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("successful action sequence replay=%#v", staleSuccess.CommandErrors)
	}

	// basic attack在 tick 14剛好離開自身 cooldown；resurrection的10秒 cooldown仍有效。
	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(3, 3, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	cooldown := rt.Step(14, 50*time.Millisecond)
	if len(cooldown.CommandErrors) != 0 || len(cooldown.ActionRejections) != 1 || !errors.Is(cooldown.ActionRejections[0].Err, combat.ErrActionCooldown) {
		t.Fatalf("resurrection cooldown=%#v", cooldown)
	}
	state, _ = rt.characters.State(2)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("cooldown rejection changed target=%#v", state)
	}
	if _, ok := policy.Pending(2); !ok {
		t.Fatal("cooldown-rejected resurrection incorrectly cancelled pending")
	}
	if err := rt.EnqueueUseAction(3, 3, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	staleCooldown := rt.Step(15, 50*time.Millisecond)
	if len(staleCooldown.CommandErrors) != 1 || !errors.Is(staleCooldown.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("cooldown action sequence replay=%#v", staleCooldown.CommandErrors)
	}
}

func makeResurrectionRuntime(t *testing.T) (*Runtime, *respawnpolicy.Service) {
	t.Helper()
	nav := navigation.Plane{MinX: -200, MaxX: 200, MinZ: -200, MaxZ: 200, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}}},
		{ID: 3, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 3, Layer: 0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{ID: "basic-attack", Effect: combat.EffectDamage, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5},
		{ID: "resurrect", Effect: combat.EffectResurrect, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, ReviveHPPercent: 30, CooldownSeconds: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := respawnpolicy.NewService(respawnpolicy.Definition{
		SchemaVersion: respawnpolicy.SchemaVersion,
		Revision:      "resurrection-test",
		SpawnPoints: []respawnpolicy.SpawnPoint{
			{ID: "safe", Class: respawnpolicy.SpawnClassSafe, X: 50, Layer: 0},
			{ID: "checkpoint", Class: respawnpolicy.SpawnClassCheckpoint, X: 100, Layer: 0, CheckpointActivationRadius: 4},
			{ID: "siege", Class: respawnpolicy.SpawnClassSiege, X: -50, Layer: 0},
		},
		Contexts: []respawnpolicy.ContextRule{
			{Context: respawnpolicy.DeathContextPvE, RespawnDelaySeconds: 0.25, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint}},
			{Context: respawnpolicy.DeathContextPvP, RespawnDelaySeconds: 0.25, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe}},
			{Context: respawnpolicy.DeathContextSiege, RespawnDelaySeconds: 0.25, DefaultSpawnPoint: "siege", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassSiege}},
		},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los: true}), WithCombatService(combatService), WithRespawnPolicy(policy))
	for id := session.ID(1); id <= 3; id++ {
		conn := session.NewQueueConnection(256, 256)
		s, err := session.New(id, world.EntityID(id), 200, conn)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(s); err != nil {
			t.Fatal(err)
		}
	}
	return rt, policy
}
