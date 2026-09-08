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

func TestShadowbladeDualBladeBuildsFlawFromSideAndRespectsBuildICD(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeDualBladeRuntime(t, world.Position{X: 3, Layer: 0}, 0)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	intent := protocol.ClientUseAction{ActionID: classaction.ShadowbladeDualBladeStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9603"}

	if err := rt.EnqueueUseAction(s.ID, 1, intent); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("first hit report=%#v", report) }
	state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if !ok || state.Current != 1 || state.Max != 3 || state.ReadyTick != 52 { t.Fatalf("first flaw=%+v ok=%v", state, ok) }
	found := false
	for _, envelope := range drainReliable(conn) {
		if resource, ok := envelope.Message.(protocol.CharacterTargetResourceState); ok {
			found = true
			if resource.SourceEntityID != s.EntityID || resource.TargetEntityID != targetID || resource.ResourceID != "flaw" || resource.Current != 1 || resource.Max != 3 { t.Fatalf("resource=%#v", resource) }
		}
	}
	if !found { t.Fatal("missing authoritative Flaw state") }

	if err := rt.EnqueueUseAction(s.ID, 2, intent); err != nil { t.Fatal(err) }
	report = rt.Step(22, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("second hit report=%#v", report) }
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 1 { t.Fatalf("build ICD changed flaw=%+v", state) }
	for _, envelope := range drainReliable(conn) {
		if _, ok := envelope.Message.(protocol.CharacterTargetResourceState); ok { t.Fatal("build ICD emitted duplicate Flaw state") }
	}

	if err := rt.EnqueueUseAction(s.ID, 3, intent); err != nil { t.Fatal(err) }
	report = rt.Step(52, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("third hit report=%#v", report) }
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 2 { t.Fatalf("ready hit flaw=%+v", state) }
	monster, _ := rt.combatantState(targetID)
	if monster.HP != 230 { t.Fatalf("monster hp=%d want 230", monster.HP) }
}

func TestShadowbladeDualBladeFrontHitDamagesWithoutFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeDualBladeRuntime(t, world.Position{Z: -3, Layer: 0}, 0)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{ActionID: classaction.ShadowbladeDualBladeStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9603"}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("front hit report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("front hit built Flaw") }
	monster, _ := rt.combatantState(targetID)
	if monster.HP != 410 { t.Fatalf("monster hp=%d want 410", monster.HP) }
}

func TestShadowbladeDualBladeOutOfRangeDoesNotDamageOrBuildFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeDualBladeRuntime(t, world.Position{X: 5, Layer: 0}, 0)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{ActionID: classaction.ShadowbladeDualBladeStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9603"}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 { t.Fatalf("out-of-range report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("out-of-range built Flaw") }
	monster, _ := rt.combatantState(targetID)
	if monster.HP != 500 { t.Fatalf("monster hp=%d want 500", monster.HP) }
}

func TestShadowbladeDualBladeRejectsWrongClassBeforeDamageOrFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeDualBladeRuntime(t, world.Position{X: 3, Layer: 0}, 0)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, protocol.ClientUseAction{ActionID: classaction.ShadowbladeDualBladeStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9603"}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 { t.Fatalf("wrong-class report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("wrong-class request built Flaw") }
	monster, _ := rt.combatantState(targetID)
	if monster.HP != 500 { t.Fatalf("wrong class changed monster hp=%d", monster.HP) }
	found := false
	for _, envelope := range drainReliable(conn) {
		if rejected, ok := envelope.Message.(protocol.ActionRejected); ok && rejected.ActionID == classaction.ShadowbladeDualBladeStrike {
			found = true
			if rejected.Reason != protocol.ActionRejectionWrongClass { t.Fatalf("reason=%q want wrong_class", rejected.Reason) }
		}
	}
	if !found { t.Fatal("missing wrong_class ActionRejected") }
}

func newShadowbladeDualBladeRuntime(t *testing.T, targetPosition world.Position, targetYaw float32) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{SchemaVersion: gameplayworld.SchemaVersion, WorldID: "shadowblade-dual-blade-test", Revision: "r1", Units: "meters", Agent: gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5}, Surfaces: []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20}}}}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	targetID := world.EntityID(9603)
	if err := sim.Spawn(world.EntityState{ID: targetID, Kind: world.EntityMonster, Transform: world.Transform{Position: targetPosition, Yaw: targetYaw}}, 4, .35, .5); err != nil { t.Fatal(err) }
	combatService, err := combat.NewService([]combat.ActionDefinition{{ID: classaction.ShadowbladeDualBladeStrike, Effect: combat.EffectDamage, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 90, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: .95}})
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
