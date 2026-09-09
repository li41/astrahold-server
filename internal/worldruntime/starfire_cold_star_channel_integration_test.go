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

func TestStarfireColdStarChannelReducesHeatAndMitigatesUntilExpiry(t *testing.T) {
	rt, s, conn, monsterID := newStarfireColdStarChannelRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.StarHeat, 80); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := protocol.ClientUseAction{
		ActionID:   classaction.StarfireColdStarChannel,
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "10",
	}
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("cold star report=%#v", report)
	}
	actor, ok := rt.characters.State(s.EntityID)
	if !ok || actor.ClassResourceID != classresource.StarHeat || actor.ClassResource != 35 {
		t.Fatalf("actor=%+v ok=%v want star_heat=35", actor, ok)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 2); got != 10 {
		t.Fatalf("active reduction=%d want=10", got)
	}

	foundResource := false
	foundStarted := false
	for _, envelope := range drainReliable(conn) {
		switch message := envelope.Message.(type) {
		case protocol.CharacterClassResourceState:
			if message.EntityID == s.EntityID && message.ResourceID == "star_heat" && message.Current == 35 && message.Max == 100 {
				foundResource = true
			}
		case protocol.ActionStarted:
			if message.ActorEntityID == s.EntityID && message.ActionID == classaction.StarfireColdStarChannel && message.TargetKind == protocol.ActionTargetEntity && message.TargetID == "10" {
				foundStarted = true
			}
		}
	}
	if !foundResource || !foundStarted {
		t.Fatalf("presentation resource=%v started=%v", foundResource, foundStarted)
	}

	applyColdStarTestWolfBite(t, rt, monsterID, 3)
	player, ok := rt.combatantState(s.EntityID)
	if !ok || player.HP != 910 {
		t.Fatalf("player after protected bite=%+v ok=%v want hp=910", player, ok)
	}
	if event := findColdStarTestCombatEvent(drainReliable(conn), "wolf-bite"); event == nil || event.Damage != 90 {
		t.Fatalf("protected bite event=%#v want damage=90", event)
	}

	// 2 seconds at 50ms/tick is 40 ticks. Commit tick=2 means [2,42) is protected.
	applyColdStarTestWolfBite(t, rt, monsterID, 42)
	player, ok = rt.combatantState(s.EntityID)
	if !ok || player.HP != 810 {
		t.Fatalf("player after expired bite=%+v ok=%v want hp=810", player, ok)
	}
	if event := findColdStarTestCombatEvent(drainReliable(conn), "wolf-bite"); event == nil || event.Damage != 100 {
		t.Fatalf("expired bite event=%#v want damage=100", event)
	}
}

func TestStarfireColdStarChannelClampsLowHeatToZero(t *testing.T) {
	rt, s, conn, _ := newStarfireColdStarChannelRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.StarHeat, 32); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID: classaction.StarfireColdStarChannel, TargetKind: protocol.ActionTargetEntity, TargetID: "10",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("low-heat report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 0 {
		t.Fatalf("low heat=%d want=0", actor.ClassResource)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 2); got != 10 {
		t.Fatalf("reduction=%d want=10", got)
	}
	foundZero := false
	for _, envelope := range drainReliable(conn) {
		if state, ok := envelope.Message.(protocol.CharacterClassResourceState); ok && state.ResourceID == "star_heat" {
			foundZero = state.Current == 0 && state.Max == 100
		}
	}
	if !foundZero {
		t.Fatal("missing authoritative star_heat 0/100 replacement")
	}
}

func TestStarfireColdStarChannelAtZeroHeatStillStartsDefensiveWindow(t *testing.T) {
	rt, s, conn, _ := newStarfireColdStarChannelRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID: classaction.StarfireColdStarChannel, TargetKind: protocol.ActionTargetEntity, TargetID: "10",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("zero-heat report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 0 || rt.combat.SelfDamageReductionPercent(s.EntityID, 2) != 10 {
		t.Fatalf("zero-heat accepted state actor=%+v", actor)
	}
	foundStarted := false
	for _, envelope := range drainReliable(conn) {
		if started, ok := envelope.Message.(protocol.ActionStarted); ok && started.ActionID == classaction.StarfireColdStarChannel {
			foundStarted = true
		}
	}
	if !foundStarted {
		t.Fatal("missing ActionStarted at zero heat")
	}
}

func TestStarfireColdStarChannelRejectsOtherTargetWithoutReductionOrCooldown(t *testing.T) {
	rt, s, conn, _ := newStarfireColdStarChannelRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.StarHeat, 80); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{
		ActionID: classaction.StarfireColdStarChannel, TargetKind: protocol.ActionTargetEntity, TargetID: "9504",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("other-target report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 80 || rt.combat.SelfDamageReductionPercent(s.EntityID, 2) != 0 {
		t.Fatalf("other-target rejection mutated state actor=%+v", actor)
	}
	foundInvalid := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.StarfireColdStarChannel {
			foundInvalid = rejected.Reason == protocol.ActionRejectionInvalidTarget
		}
	}
	if !foundInvalid {
		t.Fatal("missing invalid_target rejection")
	}

	if err := rt.EnqueueUseAction(s.ID, 2, protocol.ClientUseAction{
		ActionID: classaction.StarfireColdStarChannel, TargetKind: protocol.ActionTargetEntity, TargetID: "10",
	}); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("self retry report=%#v", report)
	}
	actor, _ = rt.characters.State(s.EntityID)
	if actor.ClassResource != 35 {
		t.Fatalf("self retry heat=%d want=35", actor.ClassResource)
	}
}

func TestStarfireColdStarChannelCooldownDoesNotReduceHeatAgain(t *testing.T) {
	rt, s, conn, _ := newStarfireColdStarChannelRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.StarHeat, 80); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	intent := protocol.ClientUseAction{ActionID: classaction.StarfireColdStarChannel, TargetKind: protocol.ActionTargetEntity, TargetID: "10"}
	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first use report=%#v", report)
	}
	if _, err := rt.characters.GainClassResource(s.EntityID, classresource.StarHeat, 45); err != nil {
		t.Fatal(err)
	}
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 {
		t.Fatalf("cooldown report=%#v", report)
	}
	actor, _ := rt.characters.State(s.EntityID)
	if actor.ClassResource != 80 {
		t.Fatalf("cooldown rejection reduced heat=%d want=80", actor.ClassResource)
	}
	if got := rt.combat.SelfDamageReductionPercent(s.EntityID, 42); got != 0 {
		t.Fatalf("cooldown rejection extended mitigation, reduction at expiry=%d", got)
	}
}

func newStarfireColdStarChannelRuntime(t *testing.T) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "starfire-cold-star-channel-test",
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
	monsterID := world.EntityID(9504)
	if err := sim.Spawn(world.EntityState{
		ID: monsterID, Kind: world.EntityMonster,
		Transform: world.Transform{Position: world.Position{X: 1, Layer: 0}},
	}, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: classaction.StarfireColdStarChannel, Effect: combat.EffectSelfMitigation,
			Targets: []combat.TargetKind{combat.TargetEntity}, Range: 0.1,
			SelfDamageReductionPercent: 10, DurationSeconds: 2, CooldownSeconds: 20,
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

func applyColdStarTestWolfBite(t *testing.T, rt *Runtime, monsterID world.EntityID, tick uint64) {
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

func findColdStarTestCombatEvent(envelopes []protocol.Envelope, actionID string) *protocol.CombatEvent {
	for _, envelope := range envelopes {
		if event, ok := envelope.Message.(protocol.CombatEvent); ok && event.ActionID == actionID {
			copy := event
			return &copy
		}
	}
	return nil
}
