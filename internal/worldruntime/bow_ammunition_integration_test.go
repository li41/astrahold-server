package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func newBowAmmunitionRuntime(t *testing.T, targetX float32) (*Runtime, *session.Session, *session.QueueConnection, world.EntityID) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "bow-ammunition-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -40, MaxX: 40, MinZ: -40, MaxZ: 40},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	monsterID := world.EntityID(9101)
	monsterEntity := world.EntityState{
		ID: monsterID, Kind: world.EntityMonster, ArchetypeID: "test-bow-target", BodySize: world.EntityBodySizeSmall,
		Transform: world.Transform{Position: world.Position{X: targetX, Z: 0, Layer: 0}},
	}
	if err := sim.Spawn(monsterEntity, 4, .35, .5); err != nil {
		t.Fatal(err)
	}
	combatCatalog, err := combat.NewService([]combat.ActionDefinition{{
		ID: basicAttackActionID, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
		BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: .5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	lifecycle := MonsterLifecycleConfig{
		Spawn: SpawnEntityRequest{
			Entity: monsterEntity, Speed: 4, Radius: .35, MaxStepHeight: .5,
			HP: 300, MaxHP: 300, BodySize: equipmentcatalog.BodySizeSmall,
		},
		CorpseHoldTicks: 2, RespawnDelayTicks: 8,
	}
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatCatalog), WithMonsterLifecycle(lifecycle))
	if err := rt.characters.RegisterState(character.State{EntityID: monsterID, HP: 300, MaxHP: 300, MP: 100, MaxMP: 100}); err != nil {
		t.Fatal(err)
	}

	conn := session.NewQueueConnection(256, 32)
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
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	if err := rt.characterSkills.restoreAppearance(s.EntityID, appearance.Ninja); err != nil {
		t.Fatal(err)
	}
	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add("item_hunter_shortbow", 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{
		Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: "item_hunter_shortbow",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip errors: %#v", report.CommandErrors)
	}
	return rt, s, conn, monsterID
}

func nextBowCombatEvent(t *testing.T, conn *session.QueueConnection) protocol.CombatEvent {
	t.Helper()
	for i := 0; i < 256; i++ {
		select {
		case envelope := <-conn.Reliable():
			if event, ok := envelope.Message.(protocol.CombatEvent); ok {
				return event
			}
		default:
			t.Fatal("combat event missing")
		}
	}
	t.Fatal("combat event missing")
	return protocol.CombatEvent{}
}

func TestBowWithoutArrowRejectsShotWithoutCooldownThenFiresAfterArrowArrives(t *testing.T) {
	oldAccuracyRoll := weaponAccuracyRoll
	weaponAccuracyRoll = func() uint32 { return 0 }
	t.Cleanup(func() { weaponAccuracyRoll = oldAccuracyRoll })

	rt, s, conn, monsterID := newBowAmmunitionRuntime(t, 6)
	inv := rt.inventories[s.CharacterIdentity.ID]
	attack := protocol.ClientUseAction{ActionID: basicAttackActionID, TargetKind: protocol.ActionTargetEntity, TargetID: "9101"}

	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 || !errors.Is(report.ActionRejections[0].Err, ErrBowArrowRequired) {
		t.Fatalf("no-arrow report=%#v", report)
	}
	state, ok := rt.combatantState(monsterID)
	if !ok || state.HP != 300 {
		t.Fatalf("no-arrow shot changed target state=%+v ok=%v", state, ok)
	}

	if err := inv.Add(ammunition.ItemWoodArrow, 1); err != nil {
		t.Fatal(err)
	}
	// Tick 4 is only 50ms after the rejected attempt. A bow cooldown would still be active here if
	// the no-arrow attempt had been committed.
	if err := rt.EnqueueUseAction(s.ID, 3, attack); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(4, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("arrow-present report=%#v", report)
	}
	event := nextBowCombatEvent(t, conn)
	if event.Result != protocol.CombatEventHit || event.TargetEntityID != monsterID {
		t.Fatalf("event=%#v", event)
	}
	if event.Damage < 8 || event.Damage > 11 {
		t.Fatalf("low bow+arrow damage=%d want 8..11", event.Damage)
	}
	if inv.Quantity(ammunition.ItemWoodArrow) != 0 {
		t.Fatalf("arrow quantity=%d want 0", inv.Quantity(ammunition.ItemWoodArrow))
	}
}

func TestBowMissConsumesExactlyOneArrow(t *testing.T) {
	oldAccuracyRoll := weaponAccuracyRoll
	weaponAccuracyRoll = func() uint32 { return 9999 }
	t.Cleanup(func() { weaponAccuracyRoll = oldAccuracyRoll })

	rt, s, conn, _ := newBowAmmunitionRuntime(t, 6)
	inv := rt.inventories[s.CharacterIdentity.ID]
	if err := inv.Add(ammunition.ItemWoodArrow, 2); err != nil {
		t.Fatal(err)
	}
	attack := protocol.ClientUseAction{ActionID: basicAttackActionID, TargetKind: protocol.ActionTargetEntity, TargetID: "9101"}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("miss report=%#v", report)
	}
	event := nextBowCombatEvent(t, conn)
	if event.Result != protocol.CombatEventMiss {
		t.Fatalf("event=%#v want miss", event)
	}
	if inv.Quantity(ammunition.ItemWoodArrow) != 1 {
		t.Fatalf("arrow quantity after miss=%d want 1", inv.Quantity(ammunition.ItemWoodArrow))
	}
}

func TestOutOfRangeBowAttackDoesNotConsumeArrow(t *testing.T) {
	rt, s, _, _ := newBowAmmunitionRuntime(t, 19)
	inv := rt.inventories[s.CharacterIdentity.ID]
	if err := inv.Add(ammunition.ItemWoodArrow, 1); err != nil {
		t.Fatal(err)
	}
	attack := protocol.ClientUseAction{ActionID: basicAttackActionID, TargetKind: protocol.ActionTargetEntity, TargetID: "9101"}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 {
		t.Fatalf("out-of-range report=%#v", report)
	}
	if inv.Quantity(ammunition.ItemWoodArrow) != 1 {
		t.Fatalf("out-of-range attack consumed arrow, quantity=%d", inv.Quantity(ammunition.ItemWoodArrow))
	}
}

func TestBowConsumesWoodBeforeSilver(t *testing.T) {
	oldAccuracyRoll := weaponAccuracyRoll
	weaponAccuracyRoll = func() uint32 { return 0 }
	t.Cleanup(func() { weaponAccuracyRoll = oldAccuracyRoll })

	rt, s, _, _ := newBowAmmunitionRuntime(t, 6)
	inv := rt.inventories[s.CharacterIdentity.ID]
	for _, itemID := range []string{ammunition.ItemWoodArrow, ammunition.ItemSilverArrow} {
		if err := inv.Add(itemID, 1); err != nil {
			t.Fatal(err)
		}
	}
	attack := protocol.ClientUseAction{ActionID: basicAttackActionID, TargetKind: protocol.ActionTargetEntity, TargetID: "9101"}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("attack report=%#v", report)
	}
	if inv.Quantity(ammunition.ItemWoodArrow) != 0 || inv.Quantity(ammunition.ItemSilverArrow) != 1 {
		t.Fatalf("arrow priority wood=%d silver=%d", inv.Quantity(ammunition.ItemWoodArrow), inv.Quantity(ammunition.ItemSilverArrow))
	}
}

func TestRemovedSteelAndStarsteelArrowIDsDoNotSatisfyBowRequirement(t *testing.T) {
	rt, s, _, _ := newBowAmmunitionRuntime(t, 6)
	inv := rt.inventories[s.CharacterIdentity.ID]
	for _, removed := range []string{"item_mid_arrow", "item_high_arrow", "item_starsteel_arrow"} {
		if err := inv.Add(removed, 1); err != nil {
			t.Fatal(err)
		}
	}
	attack := protocol.ClientUseAction{ActionID: basicAttackActionID, TargetKind: protocol.ActionTargetEntity, TargetID: "9101"}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 || !errors.Is(report.ActionRejections[0].Err, ErrBowArrowRequired) {
		t.Fatalf("removed-arrow report=%#v", report)
	}
}
