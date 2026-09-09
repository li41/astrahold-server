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

func TestStarfireFireBoltHitsAtRangeAndGainsStarHeat(t *testing.T) {
	rt, s, conn, monsterID := newStarfireFireBoltRuntime(t, 10)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := protocol.ClientUseAction{
		ActionID:   classaction.StarfireFireBolt,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9403",
	}
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("hit report=%#v", report)
	}
	monster, ok := rt.combatantState(monsterID)
	if !ok || monster.HP != 160 {
		t.Fatalf("monster=%+v ok=%v, want hp=160", monster, ok)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.StarHeat || actor.ClassResource != 8 || actor.MaxClassResource != 100 {
		t.Fatalf("actor resource=%+v ok=%v", actor, ok)
	}

	foundResource := false
	for _, envelope := range drainReliable(conn) {
		if state, ok := envelope.Message.(protocol.CharacterClassResourceState); ok {
			foundResource = true
			if state.EntityID != s.EntityID || state.ResourceID != "star_heat" || state.Current != 8 || state.Max != 100 {
				t.Fatalf("resource state=%#v", state)
			}
		}
	}
	if !foundResource {
		t.Fatal("missing authoritative star heat state after accepted fire bolt")
	}

	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("immediate repeat report=%#v", report)
	}
	actor, _ = rt.characters.State(s.EntityID)
	if actor.ClassResource != 8 {
		t.Fatalf("cooldown rejection changed star heat=%d", actor.ClassResource)
	}
	monster, _ = rt.combatantState(monsterID)
	if monster.HP != 160 {
		t.Fatalf("cooldown rejection changed monster hp=%d", monster.HP)
	}
}

func TestStarfireFireBoltOutOfRangeDoesNotDamageOrGainHeat(t *testing.T) {
	rt, s, conn, monsterID := newStarfireFireBoltRuntime(t, 14)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID:   classaction.StarfireFireBolt,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9403",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("out-of-range report=%#v", report)
	}
	monster, _ := rt.combatantState(monsterID)
	if monster.HP != 250 {
		t.Fatalf("out-of-range request changed monster hp=%d", monster.HP)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 0 {
		t.Fatalf("out-of-range request changed star heat=%d", actor.ClassResource)
	}
}

func TestStarfireFireBoltRejectsWrongClassBeforeDamageOrHeat(t *testing.T) {
	rt, s, conn, monsterID := newStarfireFireBoltRuntime(t, 10)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID:   classaction.StarfireFireBolt,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9403",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("wrong-class report=%#v", report)
	}
	monster, _ := rt.combatantState(monsterID)
	if monster.HP != 250 {
		t.Fatalf("wrong class changed monster hp=%d", monster.HP)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResourceID != classresource.Resolve || actor.ClassResource != 0 {
		t.Fatalf("wrong class changed actor resource=%+v", actor)
	}

	found := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.StarfireFireBolt {
			found = true
			if rejected.Reason != protocol.ActionRejectionWrongClass {
				t.Fatalf("reason=%q want wrong_class", rejected.Reason)
			}
		}
	}
	if !found {
		t.Fatal("missing wrong_class ActionRejected")
	}
}

func newStarfireFireBoltRuntime(t *testing.T, monsterX float32) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "starfire-fire-bolt-test",
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
	monsterID := world.EntityID(9403)
	if err := sim.Spawn(world.EntityState{
		ID: monsterID, Kind: world.EntityMonster,
		Transform: world.Transform{Position: world.Position{X: monsterX, Layer: 0}},
	}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:              classaction.StarfireFireBolt,
		Effect:          combat.EffectDamage,
		Targets:         []combat.TargetKind{combat.TargetEntity},
		Range:           12,
		BaseDamage:      90,
		DamageType:      combat.DamageMagic,
		CooldownSeconds: 1.25,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 250, MaxHP: 250, MP: 100, MaxMP: 100}); err != nil {
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
