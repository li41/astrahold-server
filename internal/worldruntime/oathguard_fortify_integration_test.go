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

func TestOathguardFortifySpendsResolveAndMitigatesUntilExpiry(t *testing.T) {
	rt, s, conn, monsterID := newOathguardFortifyRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Resolve, 30); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	fortify := protocol.ClientUseAction{
		ActionID:   classaction.OathguardFortify,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "10",
	}
	if err := rt.EnqueueUseAction(s.ID, 1, fortify); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("fortify report=%#v", report)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.Resolve || actor.ClassResource != 0 {
		t.Fatalf("actor=%+v ok=%v want resolve=0", actor, ok)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 2); got != 45 {
		t.Fatalf("active reduction=%d want=45", got)
	}

	foundResource := false
	foundStarted := false
	for _, envelope := range drainReliable(conn) {
		switch message := envelope.Message.(type) {
		case protocol.CharacterClassResourceState:
			if message.EntityID == s.EntityID && message.ResourceID == "resolve" && message.Current == 0 && message.Max == 100 {
				foundResource = true
			}
		case protocol.ActionStarted:
			if message.ActorEntityID == s.EntityID && message.ActionID == classaction.OathguardFortify && message.TargetKind == protocol.ActionTargetEntity && message.TargetID == "10" {
				foundStarted = true
			}
		}
	}
	if !foundResource || !foundStarted {
		t.Fatalf("fortify presentation resource=%v started=%v", foundResource, foundStarted)
	}

	applyFortifyTestWolfBite(t, rt, monsterID, 3)
	player, ok := rt.combatantState(s.EntityID)
	if !ok || player.HP != 945 {
		t.Fatalf("player after protected bite=%+v ok=%v want hp=945", player, ok)
	}
	if event := findFortifyTestCombatEvent(drainReliable(conn), "wolf-bite"); event == nil || event.Damage != 55 {
		t.Fatalf("protected bite event=%#v want damage=55", event)
	}

	applyFortifyTestWolfBite(t, rt, monsterID, 62)
	player, ok = rt.combatantState(s.EntityID)
	if !ok || player.HP != 845 {
		t.Fatalf("player after expired bite=%+v ok=%v want hp=845", player, ok)
	}
	if event := findFortifyTestCombatEvent(drainReliable(conn), "wolf-bite"); event == nil || event.Damage != 100 {
		t.Fatalf("expired bite event=%#v want damage=100", event)
	}
}

func TestOathguardFortifyInsufficientResolveDoesNotStartCooldownOrMitigation(t *testing.T) {
	rt, s, conn, _ := newOathguardFortifyRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Resolve, 29); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := protocol.ClientUseAction{ActionID: classaction.OathguardFortify, TargetKind: protocol.ActionTargetEntity, TargetID: "10"}
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("insufficient report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 29 || rt.combat.SelfDamageReductionPercent(s.EntityID, 2) != 0 {
		t.Fatalf("rejected fortify mutated state actor=%+v", actor)
	}
	foundInsufficient := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.OathguardFortify {
			foundInsufficient = rejected.Reason == protocol.ActionRejectionInsufficientResource
		}
	}
	if !foundInsufficient {
		t.Fatal("missing insufficient_resource rejection")
	}

	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Resolve, 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("retry report=%#v", report)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 3); got != 45 {
		t.Fatalf("retry reduction=%d want=45", got)
	}
}

func TestOathguardFortifyRejectsOtherTargetWithoutSpendOrCooldown(t *testing.T) {
	rt, s, conn, _ := newOathguardFortifyRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.Resolve, 30); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID: classaction.OathguardFortify, TargetKind: protocol.ActionTargetEntity, TargetID: "9104",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("other-target report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 30 || rt.combat.SelfDamageReductionPercent(s.EntityID, 2) != 0 {
		t.Fatalf("other-target rejection mutated state actor=%+v", actor)
	}
	foundInvalid := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.OathguardFortify {
			foundInvalid = rejected.Reason == protocol.ActionRejectionInvalidTarget
		}
	}
	if !foundInvalid {
		t.Fatal("missing invalid_target rejection")
	}

	if err := rt.EnqueueUseAction(s.ID, 2, protocol.ClientUseAction{
		ActionID: classaction.OathguardFortify, TargetKind: protocol.ActionTargetEntity, TargetID: "10",
	}); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("correct-self retry report=%#v", report)
	}
}

func TestOathguardFortifyRejectsWrongClass(t *testing.T) {
	rt, s, conn, _ := newOathguardFortifyRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID: classaction.OathguardFortify, TargetKind: protocol.ActionTargetEntity, TargetID: "10",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 || rt.combat.SelfDamageReductionPercent(s.EntityID, 2) != 0 {
		t.Fatalf("wrong-class report=%#v", report)
	}
	foundWrongClass := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.OathguardFortify {
			foundWrongClass = rejected.Reason == protocol.ActionRejectionWrongClass
		}
	}
	if !foundWrongClass {
		t.Fatal("missing wrong_class rejection")
	}
}

func newOathguardFortifyRuntime(t *testing.T) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "oathguard-fortify-test",
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
	monsterID := world.EntityID(9104)
	if err := sim.Spawn(world.EntityState{
		ID: monsterID, Kind: world.EntityMonster,
		Transform: world.Transform{Position: world.Position{X: 1, Layer: 0}},
	}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: classaction.OathguardFortify, Effect: combat.EffectSelfMitigation,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 0.1,
			SelfDamageReductionPercent: 45, DurationSeconds: 3, CooldownSeconds: 20,
		},
		{
			ID: "wolf-bite", Effect: combat.EffectDamage,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
			BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: 1.35,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 500, MaxHP: 500, MP: 100, MaxMP: 100}); err != nil {
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

func applyFortifyTestWolfBite(t *testing.T, rt *Runtime, monsterID world.EntityID, tick uint64) {
	t.Helper()
	prepared, err := rt.combat.Prepare(monsterID, "wolf-bite", combat.Target{Kind: combat.TargetEntity, ID: "10"}, tick)
	if err != nil {
		t.Fatal(err)
	}
	report := StepReport{Tick: tick}
	rt.dispatchPreparedAction("test-wolf-bite", 0, 0, prepared, tick, 50*time.Millisecond, &report)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("wolf bite report=%#v", report)
	}
}

func findFortifyTestCombatEvent(envelopes []protocol.Envelope, actionID string) *protocol.CombatEvent {
	for _, envelope := range envelopes {
		if event, ok := envelope.Message.(protocol.CombatEvent); ok && event.ActionID == actionID {
			copy := event
			return &copy
		}
	}
	return nil
}
