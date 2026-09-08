package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/targetresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestShadowbladeFlawExecuteConsumesCurrentFlawForCanonicalDamageTiers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flaw   uint32
		damage uint32
	}{
		{name: "one", flaw: 1, damage: 140},
		{name: "two", flaw: 2, damage: 220},
		{name: "three", flaw: 3, damage: 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, s, conn, targetID := newShadowbladeFlawExecuteRuntime(t)
			if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
			drainReliable(conn)
			seedFlaw(t, rt, s.EntityID, targetID, tc.flaw, 1, 52)

			if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
			report := rt.Step(2, 50*time.Millisecond)
			if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("execute report=%#v", report) }
			target, ok := rt.combatantState(targetID)
			if !ok || target.HP != 500-tc.damage { t.Fatalf("target=%+v ok=%v want hp=%d", target, ok, 500-tc.damage) }
			state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
			if !ok || state.Current != 0 || state.Max != 3 || state.ReadyTick != 52 { t.Fatalf("spent flaw=%+v ok=%v", state, ok) }
			if got := flawClearCount(drainReliable(conn), s.EntityID, targetID); got != 1 { t.Fatalf("flaw clear count=%d want 1", got) }
		})
	}
}

func TestShadowbladeFlawExecuteRejectsNoFlawWithoutStartingCooldown(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeFlawExecuteRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 { t.Fatalf("no-flaw report=%#v", report) }
	assertActionRejectedReason(t, drainReliable(conn), classaction.ShadowbladeFlawExecute, protocol.ActionRejectionInsufficientResource)
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 { t.Fatalf("no-flaw request changed hp=%d", target.HP) }

	seedFlaw(t, rt, s.EntityID, targetID, 1, 2, 2)
	if err := rt.EnqueueUseAction(s.ID, 2, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("post-rejection execute report=%#v", report) }
	target, _ = rt.combatantState(targetID)
	if target.HP != 360 { t.Fatalf("rejected attempt started cooldown; hp=%d want 360", target.HP) }
}

func TestShadowbladeFlawExecuteCooldownRejectionDoesNotConsumeNewFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeFlawExecuteRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	seedFlaw(t, rt, s.EntityID, targetID, 1, 1, 1)

	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("first execute report=%#v", report) }
	drainReliable(conn)
	seedFlaw(t, rt, s.EntityID, targetID, 1, 2, 2)

	if err := rt.EnqueueUseAction(s.ID, 2, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 { t.Fatalf("cooldown report=%#v", report) }
	assertActionRejectedReason(t, drainReliable(conn), classaction.ShadowbladeFlawExecute, protocol.ActionRejectionCooldown)
	state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if !ok || state.Current != 1 { t.Fatalf("cooldown rejection consumed flaw=%+v ok=%v", state, ok) }
	target, _ := rt.combatantState(targetID)
	if target.HP != 360 { t.Fatalf("cooldown rejection changed hp=%d", target.HP) }
}

func TestShadowbladeFlawExecuteRejectsWrongClassWithoutDamageOrSpend(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeFlawExecuteRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil { t.Fatal(err) }
	drainReliable(conn)
	seedFlaw(t, rt, s.EntityID, targetID, 1, 1, 1)

	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 { t.Fatalf("wrong-class report=%#v", report) }
	assertActionRejectedReason(t, drainReliable(conn), classaction.ShadowbladeFlawExecute, protocol.ActionRejectionWrongClass)
	state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if !ok || state.Current != 1 { t.Fatalf("wrong-class request consumed flaw=%+v ok=%v", state, ok) }
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 { t.Fatalf("wrong-class request changed hp=%d", target.HP) }
}

func TestShadowbladeFlawExecuteLethalHitEmitsOneClearAndRemovesIncarnationState(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeFlawExecuteRuntime(t)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	seedFlaw(t, rt, s.EntityID, targetID, 3, 1, 52)
	if _, err := rt.characters.ReduceHP(targetID, 250); err != nil { t.Fatal(err) }

	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeFlawExecuteIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("lethal execute report=%#v", report) }
	target, ok := rt.combatantState(targetID)
	if !ok || !target.Defeated || target.HP != 0 { t.Fatalf("target=%+v ok=%v", target, ok) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("defeated target retained zero-current Flaw incarnation state") }
	if got := flawClearCount(drainReliable(conn), s.EntityID, targetID); got != 1 { t.Fatalf("lethal flaw clear count=%d want 1", got) }
}

func newShadowbladeFlawExecuteRuntime(t *testing.T) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{SchemaVersion: gameplayworld.SchemaVersion, WorldID: "shadowblade-flaw-execute-test", Revision: "r1", Units: "meters", Agent: gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5}, Surfaces: []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20}}}}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	targetID := world.EntityID(9703)
	if err := sim.Spawn(world.EntityState{ID: targetID, Kind: world.EntityMonster, Transform: world.Transform{Position: world.Position{X: 3, Layer: 0}}}, 4, .35, .5); err != nil { t.Fatal(err) }
	combatService, err := combat.NewService([]combat.ActionDefinition{{ID: classaction.ShadowbladeFlawExecute, Effect: combat.EffectDamage, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 140, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 14}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: targetID, HP: 500, MaxHP: 500, MP: 100, MaxMP: 100}); err != nil { t.Fatal(err) }
	conn := session.NewQueueConnection(128, 16)
	s, err := session.New(1, 10, 64, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, Speed: 6, Radius: .35, MaxStepHeight: .5}); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }
	return rt, s, conn, targetID
}

func shadowbladeFlawExecuteIntent() protocol.ClientUseAction {
	return protocol.ClientUseAction{ActionID: classaction.ShadowbladeFlawExecute, TargetKind: protocol.ActionTargetEntity, TargetID: "9703"}
}

func seedFlaw(t *testing.T, rt *Runtime, sourceID, targetID world.EntityID, amount uint32, tick, readyTick uint64) {
	t.Helper()
	state, changed, err := rt.characters.GainTargetResource(sourceID, targetID, targetresource.Flaw, amount, 3, tick, readyTick)
	if err != nil || !changed || state.Current != amount { t.Fatalf("seed flaw state=%+v changed=%v err=%v", state, changed, err) }
}

func assertActionRejectedReason(t *testing.T, envelopes []protocol.Envelope, actionID string, want protocol.ActionRejectionReason) {
	t.Helper()
	for _, envelope := range envelopes {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == actionID {
			if rejected.Reason != want { t.Fatalf("rejection reason=%q want=%q", rejected.Reason, want) }
			return
		}
	}
	t.Fatalf("missing ActionRejected action=%q reason=%q", actionID, want)
}

func flawClearCount(envelopes []protocol.Envelope, sourceID, targetID world.EntityID) int {
	count := 0
	for _, envelope := range envelopes {
		state, ok := envelope.Message.(protocol.CharacterTargetResourceState)
		if !ok { continue }
		if state.SourceEntityID == sourceID && state.TargetEntityID == targetID && state.ResourceID == "flaw" && state.Current == 0 && state.Max == 3 { count++ }
	}
	return count
}
