package worldruntime

import (
	"errors"
	"testing"
	"time"

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

func TestRespawnPolicySchedulesCheckpointAndAppliesAfterDelay(t *testing.T) {
	rt, policy := makeRespawnPolicyRuntime(t)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueSetRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("checkpoint=%#v", report.CommandErrors)
	}
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(3, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 || defeat.Metrics.RespawnsScheduled != 1 {
		t.Fatalf("defeat=%#v", defeat)
	}
	state, _ := rt.characters.State(2)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("defeated state=%#v", state)
	}
	pending, ok := policy.Pending(2)
	if !ok || pending.SpawnPointID != "checkpoint" || pending.DueTick != 5 {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}

	// 死後改 checkpoint 只影響下一次死亡；本次 schedule 已在 defeat tick 綁定目的地。
	if err := rt.EnqueueSetRespawnCheckpoint(2, ""); err != nil {
		t.Fatal(err)
	}
	beforeDue := rt.Step(4, 50*time.Millisecond)
	if len(beforeDue.CommandErrors) != 0 || beforeDue.Metrics.RespawnsApplied != 0 {
		t.Fatalf("before due=%#v", beforeDue)
	}
	pending, ok = policy.Pending(2)
	if !ok || pending.SpawnPointID != "checkpoint" {
		t.Fatalf("pending changed after checkpoint clear=%#v ok=%v", pending, ok)
	}

	// due tick 的 ClientMoveInput 先以 Defeated 規則 consume/zero，之後才 respawn；
	// 因此 simulation 不會把角色從新出生點沿該方向推走。
	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	due := rt.Step(5, 50*time.Millisecond)
	if len(due.CommandErrors) != 0 || due.Metrics.RespawnPolicyDue != 1 || due.Metrics.RespawnsApplied != 1 {
		t.Fatalf("due=%#v", due)
	}
	state, _ = rt.characters.State(2)
	if state.Defeated || state.HP != 200 {
		t.Fatalf("revived state=%#v", state)
	}
	entity, ok := rt.world.Entity(2)
	want := world.Position{X: 100, Y: 0, Z: 0, Layer: 0}
	if !ok || entity.Transform.Position != want {
		t.Fatalf("respawn position=%#v ok=%v", entity.Transform.Position, ok)
	}
	s2, ok := rt.sessions.Get(2)
	if !ok || s2.LastProcessedInputSequence() != 1 {
		t.Fatalf("input sequence=%#v ok=%v", s2, ok)
	}
	if _, ok := policy.Pending(2); ok {
		t.Fatal("pending respawn survived successful policy respawn")
	}

	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	stale := rt.Step(6, 50*time.Millisecond)
	if len(stale.CommandErrors) != 1 || !errors.Is(stale.CommandErrors[0].Err, session.ErrStaleInput) {
		t.Fatalf("stale input errors=%#v", stale.CommandErrors)
	}
}

func TestManualRespawnCancelsPolicyAndLeaveRemovesCheckpoint(t *testing.T) {
	rt, policy := makeRespawnPolicyRuntime(t)
	rt.Step(1, 50*time.Millisecond)
	if err := rt.EnqueueSetRespawnCheckpoint(2, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	rt.Step(2, 50*time.Millisecond)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	rt.Step(3, 50*time.Millisecond)
	if _, ok := policy.Pending(2); !ok {
		t.Fatal("missing scheduled respawn")
	}

	manual := world.Position{X: 75, Layer: 0}
	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: manual}); err != nil {
		t.Fatal(err)
	}
	manualReport := rt.Step(4, 50*time.Millisecond)
	if len(manualReport.CommandErrors) != 0 || manualReport.Metrics.RespawnsApplied != 1 {
		t.Fatalf("manual respawn=%#v", manualReport)
	}
	if _, ok := policy.Pending(2); ok {
		t.Fatal("manual respawn did not cancel pending policy")
	}
	if report := rt.Step(5, 50*time.Millisecond); report.Metrics.RespawnPolicyDue != 0 || report.Metrics.RespawnsApplied != 0 {
		t.Fatalf("cancelled policy fired=%#v", report.Metrics)
	}

	if err := rt.EnqueueLeave(2); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(6, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave=%#v", report.CommandErrors)
	}
	if _, ok := policy.Checkpoint(2); ok {
		t.Fatal("checkpoint survived entity leave")
	}
}

func makeRespawnPolicyRuntime(t *testing.T) (*Runtime, *respawnpolicy.Service) {
	t.Helper()
	nav := navigation.Plane{MinX: -200, MaxX: 200, MinZ: -200, MaxZ: 200, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "kill", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
		BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := respawnpolicy.NewService(respawnpolicy.Definition{
		SchemaVersion:       respawnpolicy.SchemaVersion,
		Revision:            "runtime-test",
		RespawnDelaySeconds: 0.1,
		DefaultSpawnPoint:   "default",
		SpawnPoints: []respawnpolicy.SpawnPoint{
			{ID: "default", X: 50, Layer: 0},
			{ID: "checkpoint", X: 100, Layer: 0},
		},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los: true}), WithCombatService(combatService), WithRespawnPolicy(policy))
	for id := session.ID(1); id <= 2; id++ {
		conn := session.NewQueueConnection(128, 128)
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
