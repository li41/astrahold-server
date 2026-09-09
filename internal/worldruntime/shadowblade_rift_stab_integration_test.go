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

func TestShadowbladeRiftStabSideHitBuildsFlawAndCooldownRejectsWithoutMutation(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{X: 3, Layer: 0}, 0, 500)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("Rift Stab report=%#v", report) }
	state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if !ok || state.Current != 1 || state.Max != 3 || state.ReadyTick != 0 { t.Fatalf("flaw=%+v ok=%v", state, ok) }
	target, _ := rt.combatantState(targetID)
	if target.HP != 365 { t.Fatalf("target hp=%d want 365", target.HP) }
	found := false
	for _, envelope := range drainReliable(conn) {
		if resource, ok := envelope.Message.(protocol.CharacterTargetResourceState); ok && resource.ResourceID == "flaw" {
			found = true
			if resource.SourceEntityID != s.EntityID || resource.TargetEntityID != targetID || resource.Current != 1 || resource.Max != 3 { t.Fatalf("resource=%#v", resource) }
		}
	}
	if !found { t.Fatal("missing authoritative Flaw state") }

	if err := rt.EnqueueUseAction(s.ID, 2, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 { t.Fatalf("cooldown report=%#v", report) }
	assertActionRejectedReason(t, drainReliable(conn), classaction.ShadowbladeRiftStab, protocol.ActionRejectionCooldown)
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 1 || state.ReadyTick != 0 { t.Fatalf("cooldown rejection mutated flaw=%+v", state) }
	target, _ = rt.combatantState(targetID)
	if target.HP != 365 { t.Fatalf("cooldown rejection changed hp=%d", target.HP) }
}

func TestShadowbladeRiftStabFrontHitDamagesWithoutFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{Z: -3, Layer: 0}, 0, 500)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("front report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("front hit built Flaw") }
	target, _ := rt.combatantState(targetID)
	if target.HP != 365 { t.Fatalf("target hp=%d want 365", target.HP) }
	for _, envelope := range drainReliable(conn) {
		if _, ok := envelope.Message.(protocol.CharacterTargetResourceState); ok { t.Fatal("front hit emitted target-resource state") }
	}
}

func TestShadowbladeRiftStabOutOfRangeDoesNotDamageOrBuildFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{X: 5, Layer: 0}, 0, 500)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 { t.Fatalf("out-of-range report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("out-of-range request built Flaw") }
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 { t.Fatalf("out-of-range request changed hp=%d", target.HP) }
}

func TestShadowbladeRiftStabRejectsWrongClassBeforeDamageOrFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{X: 3, Layer: 0}, 0, 500)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Oathguard); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 { t.Fatalf("wrong-class report=%#v", report) }
	assertActionRejectedReason(t, drainReliable(conn), classaction.ShadowbladeRiftStab, protocol.ActionRejectionWrongClass)
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("wrong-class request built Flaw") }
	target, _ := rt.combatantState(targetID)
	if target.HP != 500 { t.Fatalf("wrong-class request changed hp=%d", target.HP) }
}

func TestShadowbladeRiftStabLethalHitDoesNotCreateFlaw(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{X: 3, Layer: 0}, 0, 100)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	if err := rt.EnqueueUseAction(s.ID, 1, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("lethal report=%#v", report) }
	target, ok := rt.combatantState(targetID)
	if !ok || !target.Defeated || target.HP != 0 { t.Fatalf("target=%+v ok=%v", target, ok) }
	if _, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw); ok { t.Fatal("lethal Rift Stab created Flaw") }
	for _, envelope := range drainReliable(conn) {
		if _, ok := envelope.Message.(protocol.CharacterTargetResourceState); ok { t.Fatal("lethal Rift Stab emitted new target-resource state") }
	}
}

func TestShadowbladeRiftStabPreservesDualBladeBuildReadyTick(t *testing.T) {
	rt, s, conn, targetID := newShadowbladeRiftStabRuntime(t, world.Position{X: 3, Layer: 0}, 0, 500)
	if _, err := rt.characters.AssignInitialClass(s.EntityID, classid.Shadowblade); err != nil { t.Fatal(err) }
	drainReliable(conn)
	dual := protocol.ClientUseAction{ActionID: classaction.ShadowbladeDualBladeStrike, TargetKind: protocol.ActionTargetEntity, TargetID: "9803"}

	if err := rt.EnqueueUseAction(s.ID, 1, dual); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("Dual Blade seed report=%#v", report) }
	state, ok := rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if !ok || state.Current != 1 || state.ReadyTick != 52 { t.Fatalf("seed flaw=%+v ok=%v", state, ok) }
	drainReliable(conn)

	if err := rt.EnqueueUseAction(s.ID, 2, shadowbladeRiftStabIntent()); err != nil { t.Fatal(err) }
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("Rift Stab report=%#v", report) }
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 2 || state.ReadyTick != 52 { t.Fatalf("Rift Stab reset/extended Dual Blade ReadyTick: %+v", state) }

	if err := rt.EnqueueUseAction(s.ID, 3, dual); err != nil { t.Fatal(err) }
	report = rt.Step(22, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("Dual Blade during build ICD report=%#v", report) }
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 2 || state.ReadyTick != 52 { t.Fatalf("Rift Stab bypassed Dual Blade build ICD: %+v", state) }

	if err := rt.EnqueueUseAction(s.ID, 4, dual); err != nil { t.Fatal(err) }
	report = rt.Step(52, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("Dual Blade ready report=%#v", report) }
	state, _ = rt.characters.TargetResourceState(s.EntityID, targetID, targetresource.Flaw)
	if state.Current != 3 || state.ReadyTick != 102 { t.Fatalf("Dual Blade did not resume at original ReadyTick: %+v", state) }
	target, _ := rt.combatantState(targetID)
	if target.HP != 95 { t.Fatalf("target hp=%d want 95", target.HP) }
}

func newShadowbladeRiftStabRuntime(t *testing.T, targetPosition world.Position, targetYaw float32, targetHP uint32) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{SchemaVersion: gameplayworld.SchemaVersion, WorldID: "shadowblade-rift-stab-test", Revision: "r1", Units: "meters", Agent: gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5}, Surfaces: []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20}}}}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	targetID := world.EntityID(9803)
	if err := sim.Spawn(world.EntityState{ID: targetID, Kind: world.EntityMonster, Transform: world.Transform{Position: targetPosition, Yaw: targetYaw}}, 4, .35, .5); err != nil { t.Fatal(err) }
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{ID: classaction.ShadowbladeDualBladeStrike, Effect: combat.EffectDamage, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 90, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: .95},
		{ID: classaction.ShadowbladeRiftStab, Effect: combat.EffectDamage, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 135, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 7},
	})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatService))
	if err := rt.characters.RegisterState(character.State{EntityID: targetID, HP: targetHP, MaxHP: 500, MP: 100, MaxMP: 100}); err != nil { t.Fatal(err) }
	conn := session.NewQueueConnection(128, 16)
	s, err := session.New(1, 10, 64, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, Speed: 6, Radius: .35, MaxStepHeight: .5}); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }
	return rt, s, conn, targetID
}

func shadowbladeRiftStabIntent() protocol.ClientUseAction {
	return protocol.ClientUseAction{ActionID: classaction.ShadowbladeRiftStab, TargetKind: protocol.ActionTargetEntity, TargetID: "9803"}
}
