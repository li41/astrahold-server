package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
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

func TestEquippedGuardShieldMitigatesAuthoritativeWolfBite(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "shield-mitigation-test",
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
	monsterID := world.EntityID(9001)
	monsterEntity := world.EntityState{
		ID: monsterID, Kind: world.EntityMonster, ArchetypeID: "test-wolf",
		Transform: world.Transform{Position: world.Position{X: 1.5, Z: 0, Layer: 0}},
	}
	if err := sim.Spawn(monsterEntity, 4, .35, .5); err != nil {
		t.Fatal(err)
	}

	combatCatalog, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
		BaseDamage: 90, DamageType: combat.DamagePhysical, Blockable: true, CooldownSeconds: 1.35,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatCatalog))
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
		Entity:  world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed:   6, Radius: .35, MaxStepHeight: .5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}

	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add("item_guard_shield", 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotOffHand,
		ItemArchetypeID: "item_guard_shield",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip errors: %#v", report.CommandErrors)
	}
	if inv.OffHand() != "item_guard_shield" {
		t.Fatalf("off hand = %q", inv.OffHand())
	}

	before, ok := rt.combatantState(s.EntityID)
	if !ok {
		t.Fatal("player combatant missing")
	}
	prepared, err := combatCatalog.Prepare(monsterID, "wolf-bite", combat.Target{Kind: combat.TargetEntity, ID: "10"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	report := StepReport{Tick: 3}
	rt.dispatchPreparedAction("test-wolf-bite", 0, 0, prepared, 3, 50*time.Millisecond, &report)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("wolf bite report=%#v", report)
	}

	event := waitForCombatEvent(t, conn)
	if event.ActorEntityID != monsterID || event.TargetEntityID != s.EntityID || event.ActionID != "wolf-bite" || event.Result != protocol.CombatEventHit {
		t.Fatalf("event=%#v", event)
	}
	if event.Blocked {
		if event.Damage != 53 {
			t.Fatalf("blocked guard-shield damage=%d, want 53", event.Damage)
		}
	} else if event.Damage != 75 {
		t.Fatalf("unblocked guard-shield damage=%d, want 75", event.Damage)
	}
	after, ok := rt.combatantState(s.EntityID)
	if !ok || after.HP != before.HP-event.Damage {
		t.Fatalf("player hp after bite=%+v ok=%v before=%+v event=%#v", after, ok, before, event)
	}
}
