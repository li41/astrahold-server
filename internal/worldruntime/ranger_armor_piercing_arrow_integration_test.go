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

func TestRangerArmorPiercingArrowSpendsHuntMomentumAndDealsCanonicalBaseDamage(t *testing.T) {
	rt, s, conn, targetID := newRangerArmorPiercingArrowRuntime(t, 10)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.HuntMomentum, 30); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := rangerArmorPiercingArrowIntent()
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("piercing report=%#v", report)
	}
	target, ok := rt.combatantState(targetID)
	if !ok || target.HP != 290 {
		t.Fatalf("target=%+v ok=%v want hp=290", target, ok)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.HuntMomentum || actor.ClassResource != 0 || actor.MaxClassResource != 100 {
		t.Fatalf("actor=%+v ok=%v", actor, ok)
	}
	foundZero := false
	for _, envelope := range drainReliable(conn) {
		state, ok := envelope.Message.(protocol.CharacterClassResourceState)
		if !ok || state.EntityID != s.EntityID || state.ResourceID != "hunt_momentum" {
			continue
		}
		foundZero = true
		if state.Current != 0 || state.Max != 100 {
			t.Fatalf("resource state=%#v", state)
		}
	}
	if !foundZero {
		t.Fatal("missing authoritative zero Hunt Momentum state")
	}

	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.HuntMomentum, 30); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("cooldown repeat report=%#v", report)
	}
	actor, _ = rt.characters.State(s.EntityID)
	if actor.ClassResource != 30 {
		t.Fatalf("cooldown rejection spent Hunt Momentum=%d", actor.ClassResource)
	}
	target, _ = rt.combatantState(targetID)
	if target.HP != 290 {
		t.Fatalf("cooldown rejection changed target hp=%d", target.HP)
	}
}

func TestRangerArmorPiercingArrowInsufficientResourceDoesNotStartCooldown(t *testing.T) {
	rt, s, conn, targetID := newRangerArmorPiercingArrowRuntime(t, 10)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)
	intent := rangerArmorPiercingArrowIntent()

	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 {
		t.Fatalf("insufficient report=%#v", report)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.HuntMomentum, 30); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("post-insufficient report=%#v", report)
	}
	target, _ := rt.combatantState(targetID)
	if target.HP != 290 {
		t.Fatalf("insufficient request started cooldown; hp=%d want 290", target.HP)
	}
}

func TestRangerArmorPiercingArrowOutOfRangeDoesNotSpend(t *testing.T) {
	rt, s, conn, targetID := newRangerArmorPiercingArrowRuntime(t, 14)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.HuntMomentum, 30); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, rangerArmorPiercingArrowIntent()); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("out-of-range report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 30 {
		t.Fatalf("out-of-range request spent Hunt Momentum=%d", actor.ClassResource)
	}
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 {
		t.Fatalf("out-of-range request changed target hp=%d", target.HP)
	}
}

func TestRangerArmorPiercingArrowRejectsWrongClass(t *testing.T) {
	rt, s, conn, targetID := newRangerArmorPiercingArrowRuntime(t, 10)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, rangerArmorPiercingArrowIntent()); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("wrong-class report=%#v", report)
	}
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 {
		t.Fatalf("wrong-class request changed target hp=%d", target.HP)
	}
}

func newRangerArmorPiercingArrowRuntime(t *testing.T, targetX float32) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "ranger-armor-piercing-arrow-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	targetID := world.EntityID(9813)
	if err := sim.Spawn(world.EntityState{ID: targetID, Kind: world.EntityMonster, Transform: world.Transform{Position: world.Position{X: targetX, Layer: 0}}}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:                           classaction.RangerArmorPiercingArrow,
		Effect:                       combat.EffectDamage,
		Targets:                      []combat.TargetKind{combat.TargetEntity},
		Range:                        12,
		BaseDamage:                   210,
		DamageType:                   combat.DamagePhysical,
		Blockable:                    true,
		PhysicalDefenseIgnorePercent: 25,
		CooldownSeconds:              7,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: targetID, HP: 500, MaxHP: 500, MP: 100, MaxMP: 100}); err != nil {
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
	return rt, s, conn, targetID
}

func rangerArmorPiercingArrowIntent() protocol.ClientUseAction {
	return protocol.ClientUseAction{ActionID: classaction.RangerArmorPiercingArrow, TargetKind: protocol.ActionTargetEntity, TargetID: "9813"}
}
