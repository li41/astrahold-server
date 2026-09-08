package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBreakerStaggerStrikeClosesBuildSpendLoopAndCooldownDoesNotDoubleSpend(t *testing.T) {
	rt, s, conn, monsterID := newBreakerStaggerStrikeRuntime(t, world.Position{X: 2, Layer: 0})
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	heavy := protocol.ClientUseAction{ActionID: classaction.BreakerHeavySlash, TargetKind: protocol.ActionTargetEntity, TargetID: "9802"}
	stagger := protocol.ClientUseAction{ActionID: classaction.BreakerStaggerStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9802"}

	if err := rt.EnqueueUseAction(s.ID, 1, heavy); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first heavy slash report=%#v", report)
	}
	if err := rt.EnqueueUseAction(s.ID, 2, heavy); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(29, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("second heavy slash report=%#v", report)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.Momentum || actor.ClassResource != 20 {
		t.Fatalf("actor before stagger=%+v ok=%v, want momentum 20", actor, ok)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 3, stagger); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(30, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("stagger strike report=%#v", report)
	}
	monster, ok := rt.combatantState(monsterID)
	if !ok || monster.HP != 340 {
		t.Fatalf("monster after build-spend=%+v ok=%v, want hp=340", monster, ok)
	}
	actor, _ = rt.characters.State(s.EntityID)
	if actor.ClassResource != 0 {
		t.Fatalf("momentum after stagger=%d, want 0", actor.ClassResource)
	}
	foundZero := false
	for _, envelope := range drainReliable(conn) {
		if state, ok := envelope.Message.(protocol.CharacterClassResourceState); ok && state.EntityID == s.EntityID && state.ResourceID == "momentum" {
			foundZero = true
			if state.Current != 0 || state.Max != 100 {
				t.Fatalf("spent momentum state=%#v", state)
			}
		}
	}
	if !foundZero {
		t.Fatal("missing authoritative momentum=0 state after stagger strike")
	}

	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Momentum, 20); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(s.ID, 4, stagger); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(31, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("cooldown repeat report=%#v", report)
	}
	actor, _ = rt.characters.State(s.EntityID)
	if actor.ClassResource != 20 {
		t.Fatalf("cooldown rejection spent momentum=%d, want 20", actor.ClassResource)
	}
	monster, _ = rt.combatantState(monsterID)
	if monster.HP != 340 {
		t.Fatalf("cooldown rejection changed monster hp=%d", monster.HP)
	}
}

func TestBreakerStaggerStrikeInsufficientMomentumDoesNotStartCooldown(t *testing.T) {
	rt, s, conn, monsterID := newBreakerStaggerStrikeRuntime(t, world.Position{X: 2, Layer: 0})
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)
	intent := protocol.ClientUseAction{ActionID: classaction.BreakerStaggerStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9802"}

	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("insufficient report=%#v", report)
	}
	foundInsufficient := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.BreakerStaggerStrike {
			foundInsufficient = true
			if rejected.Reason != protocol.ActionRejectionInsufficientResource {
				t.Fatalf("reason=%q want insufficient_resource", rejected.Reason)
			}
		}
	}
	if !foundInsufficient {
		t.Fatal("missing insufficient_resource ActionRejected")
	}
	monster, _ := rt.combatantState(monsterID)
	if monster.HP != 800 {
		t.Fatalf("insufficient request changed monster hp=%d", monster.HP)
	}

	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Momentum, 20); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("post-seed stagger report=%#v", report)
	}
	monster, _ = rt.combatantState(monsterID)
	if monster.HP != 610 {
		t.Fatalf("post-seed monster hp=%d, want 610", monster.HP)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 0 {
		t.Fatalf("post-seed stagger momentum=%d, want 0", actor.ClassResource)
	}
}

func TestBreakerStaggerStrikeOutOfRangeDoesNotSpendMomentum(t *testing.T) {
	rt, s, conn, monsterID := newBreakerStaggerStrikeRuntime(t, world.Position{X: 5, Layer: 0})
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Momentum, 20); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{ActionID: classaction.BreakerStaggerStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9802"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("out-of-range report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 20 {
		t.Fatalf("out-of-range spent momentum=%d, want 20", actor.ClassResource)
	}
	monster, _ := rt.combatantState(monsterID)
	if monster.HP != 800 {
		t.Fatalf("out-of-range changed monster hp=%d", monster.HP)
	}
}

func newBreakerStaggerStrikeRuntime(t *testing.T, targetPosition world.Position) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "breaker-stagger-strike-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	monsterID := world.EntityID(9802)
	if err := sim.Spawn(world.EntityState{ID: monsterID, Kind: world.EntityMonster, Transform: world.Transform{Position: targetPosition}}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: classaction.BreakerHeavySlash, Effect: combat.EffectDamage,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
			BaseDamage: 135, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 1.35,
		},
		{
			ID: classaction.BreakerStaggerStrike, Effect: combat.EffectDamage,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
			BaseDamage: 190, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 800, MaxHP: 800, MP: 100, MaxMP: 100}); err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(128, 16)
	s, err := session.New(1, 10, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: .35, MaxStepHeight: .5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	return rt, s, conn, monsterID
}
