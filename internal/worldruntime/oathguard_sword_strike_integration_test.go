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

func TestOathguardSwordStrikeHitGainsResolveAndCooldownDoesNot(t *testing.T) {
	rt, s, conn, monsterID := newOathguardSwordStrikeRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := protocol.ClientUseAction{
		ActionID:   classaction.OathguardSwordStrike,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9102",
	}
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("hit report=%#v", report)
	}
	monster, ok := rt.combatantState(monsterID)
	if !ok || monster.HP != 100 {
		t.Fatalf("monster=%+v ok=%v, want hp=100", monster, ok)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.Resolve || actor.ClassResource != 8 || actor.MaxClassResource != 100 {
		t.Fatalf("actor resource=%+v ok=%v", actor, ok)
	}

	foundResource := false
	for _, envelope := range drainReliable(conn) {
		if state, ok := envelope.Message.(protocol.CharacterClassResourceState); ok {
			foundResource = true
			if state.EntityID != s.EntityID || state.ResourceID != "resolve" || state.Current != 8 || state.Max != 100 {
				t.Fatalf("resource state=%#v", state)
			}
		}
	}
	if !foundResource {
		t.Fatal("missing authoritative class resource state after hit")
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
		t.Fatalf("cooldown rejection changed resolve=%d", actor.ClassResource)
	}
	monster, _ = rt.combatantState(monsterID)
	if monster.HP != 100 {
		t.Fatalf("cooldown rejection changed monster hp=%d", monster.HP)
	}
}

func TestOathguardSwordStrikeRejectsWrongClassBeforeDamage(t *testing.T) {
	rt, s, conn, monsterID := newOathguardSwordStrikeRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID:   classaction.OathguardSwordStrike,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9102",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("wrong-class report=%#v", report)
	}
	monster, _ := rt.combatantState(monsterID)
	if monster.HP != 200 {
		t.Fatalf("wrong class changed monster hp=%d", monster.HP)
	}

	found := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.OathguardSwordStrike {
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

func newOathguardSwordStrikeRuntime(t *testing.T) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "oathguard-sword-strike-test",
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
	monsterID := world.EntityID(9102)
	if err := sim.Spawn(world.EntityState{
		ID: monsterID, Kind: world.EntityMonster,
		Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}},
	}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:              classaction.OathguardSwordStrike,
		Effect:          combat.EffectDamage,
		Targets:         []combat.TargetKind{combat.TargetEntity},
		Range:           4.5,
		BaseDamage:      100,
		DamageType:      combat.DamagePhysical,
		Blockable:       true,
		CooldownSeconds: 1.1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 200, MaxHP: 200, MP: 100, MaxMP: 100}); err != nil {
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
