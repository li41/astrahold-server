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
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRespawnRestoresFullHPRelocatesAndPreservesInputSequence(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", report.CommandErrors)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first hit=%#v", report)
	}
	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(12, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", report)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	// Defeated 期間收到的 move sequence 必須先被 consume；respawn 不重置 sequence history。
	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	respawnAt := world.Position{X: 50, Y: 0, Z: 0, Layer: 0}
	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: respawnAt}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(13, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.RespawnsApplied != 1 {
		t.Fatalf("respawn report=%#v", report)
	}
	state, ok := rt.characters.State(2)
	if !ok || state.HP != 200 || state.MaxHP != 200 || state.Defeated {
		t.Fatalf("revived state=%#v ok=%v", state, ok)
	}
	entity, ok := rt.world.Entity(2)
	if !ok || entity.Transform.Position != respawnAt {
		t.Fatalf("respawn position=%#v ok=%v", entity.Transform.Position, ok)
	}
	s2, ok := rt.sessions.Get(2)
	if !ok || s2.LastProcessedInputSequence() != 1 {
		t.Fatalf("input history=%#v ok=%v", s2, ok)
	}

	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	stale := rt.Step(14, 50*time.Millisecond)
	if len(stale.CommandErrors) != 1 || !errors.Is(stale.CommandErrors[0].Err, session.ErrStaleInput) {
		t.Fatalf("stale move errors=%#v", stale.CommandErrors)
	}
	still, _ := rt.world.Entity(2)
	if still.Transform.Position != respawnAt {
		t.Fatalf("stale move changed position: %#v", still.Transform.Position)
	}

	if err := rt.EnqueueMove(2, 2, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	fresh := rt.Step(15, 50*time.Millisecond)
	if len(fresh.CommandErrors) != 0 {
		t.Fatalf("fresh move errors=%#v", fresh.CommandErrors)
	}
	moved, _ := rt.world.Entity(2)
	if moved.Transform.Position.X <= respawnAt.X {
		t.Fatalf("fresh post-respawn move did not advance: %#v", moved.Transform.Position)
	}

	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: world.Position{X: 75, Layer: 0}}); err != nil {
		t.Fatal(err)
	}
	alive := rt.Step(16, 50*time.Millisecond)
	if len(alive.CommandErrors) != 1 || !errors.Is(alive.CommandErrors[0].Err, character.ErrCharacterNotDefeated) {
		t.Fatalf("alive respawn errors=%#v", alive.CommandErrors)
	}
}

type respawnTestConnection struct {
	reliable            []protocol.Envelope
	realtime            []protocol.Envelope
	backpressureDespawn bool
}

func (c *respawnTestConnection) TrySend(envelope protocol.Envelope) error {
	if c.backpressureDespawn {
		if _, ok := envelope.Message.(protocol.EntityDespawn); ok {
			return session.ErrBackpressure
		}
	}
	switch envelope.Delivery {
	case protocol.DeliveryReliableOrdered:
		c.reliable = append(c.reliable, envelope)
	case protocol.DeliveryRealtimeSequenced:
		c.realtime = append(c.realtime, envelope)
	}
	return nil
}

func (c *respawnTestConnection) Close() error { return nil }

func (c *respawnTestConnection) clear() {
	c.reliable = c.reliable[:0]
	c.realtime = c.realtime[:0]
}

func (c *respawnTestConnection) hasVitals(entityID world.EntityID, hp uint32, defeated bool) bool {
	for _, envelope := range c.reliable {
		state, ok := envelope.Message.(protocol.EntityVitalsState)
		if ok && state.EntityID == entityID && state.HP == hp && state.Defeated == defeated {
			return true
		}
	}
	return false
}

func TestRespawnVitalsWaitForAOIAndExcludeStaleKnownObservers(t *testing.T) {
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
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 2
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los: true}), WithCombatService(combatService))
	oldObserver := &respawnTestConnection{}
	self := &respawnTestConnection{}
	s1, err := session.New(1, 1, 20, oldObserver)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := session.New(2, 2, 20, self)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s2); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap snapshot errors=%#v", report.CommandErrors)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap vitals errors=%#v", report.CommandErrors)
	}
	oldObserver.clear()
	self.clear()

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(4, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", defeat)
	}
	if !oldObserver.hasVitals(2, 0, true) || !self.hasVitals(2, 0, true) {
		t.Fatalf("missing defeated vitals old=%#v self=%#v", oldObserver.reliable, self.reliable)
	}
	oldObserver.clear()
	self.clear()

	respawnAt := world.Position{X: 100, Layer: 0}
	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: respawnAt}); err != nil {
		t.Fatal(err)
	}
	nonSnapshot := rt.Step(5, 50*time.Millisecond)
	if len(nonSnapshot.CommandErrors) != 0 || nonSnapshot.Metrics.RespawnsApplied != 1 {
		t.Fatalf("non-snapshot respawn=%#v", nonSnapshot)
	}
	if oldObserver.hasVitals(2, 200, false) || self.hasVitals(2, 200, false) {
		t.Fatalf("revived vitals escaped before AOI reconciliation old=%#v self=%#v", oldObserver.reliable, self.reliable)
	}
	if rt.respawnVitalsPhases[2] != respawnVitalsAwaitingAOI {
		t.Fatalf("phase before snapshot=%v", rt.respawnVitalsPhases[2])
	}
	oldObserver.clear()
	self.clear()

	// 讓舊 observer 的 Despawn 暫時 backpressure。Replication desired 已更新為 false，
	// 但 known + Vitals mirror 仍保留；這正是 S3-F.2 desired-only barrier 要封住的窗口。
	oldObserver.backpressureDespawn = true
	reconcile := rt.Step(6, 50*time.Millisecond)
	if reconcile.Metrics.LifecycleBackpressureStops == 0 {
		t.Fatalf("expected despawn backpressure, report=%#v", reconcile)
	}
	if oldObserver.hasVitals(2, 200, false) {
		t.Fatalf("stale-known observer received revived vitals: %#v", oldObserver.reliable)
	}
	if !self.hasVitals(2, 200, false) {
		t.Fatalf("current desired self missing revived vitals: %#v", self.reliable)
	}
	if rt.respawnVitalsPhases[2] != respawnVitalsDesiredOnly {
		t.Fatalf("phase with stale known=%v", rt.respawnVitalsPhases[2])
	}
	oldObserver.clear()
	self.clear()

	// 即使 revive 後又產生新的 HP revision，只要 stale Despawn 尚未確認，舊 observer
	// 仍不能重新進入一般 mirror hot path。
	if _, err := rt.characters.ReduceHP(2, 50); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(2)
	postRespawnDamage := rt.Step(7, 50*time.Millisecond)
	if len(postRespawnDamage.CommandErrors) != 0 {
		t.Fatalf("post-respawn damage report=%#v", postRespawnDamage)
	}
	if oldObserver.hasVitals(2, 150, false) {
		t.Fatalf("stale-known observer received later dirty vitals: %#v", oldObserver.reliable)
	}
	if !self.hasVitals(2, 150, false) {
		t.Fatalf("desired self missing later dirty vitals: %#v", self.reliable)
	}
	oldObserver.clear()
	self.clear()

	oldObserver.backpressureDespawn = false
	if report := rt.Step(8, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("despawn recovery errors=%#v", report.CommandErrors)
	}
	if _, guarded := rt.respawnVitalsPhases[2]; guarded {
		t.Fatalf("respawn desired-only guard did not clear after Despawn confirmation")
	}
}
