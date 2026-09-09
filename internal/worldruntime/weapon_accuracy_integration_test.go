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

func TestLowTierWeaponMissIsAuthoritativeOutcomeAndCommitsCooldown(t *testing.T) {
	oldAccuracyRoll := weaponAccuracyRoll
	weaponAccuracyRoll = func() uint32 { return 94 } // light guard sword is 94%; boundary is a miss.
	t.Cleanup(func() { weaponAccuracyRoll = oldAccuracyRoll })

	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "weapon-accuracy-test",
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
	monsterID := world.EntityID(9101)
	monsterEntity := world.EntityState{
		ID:          monsterID,
		Kind:        world.EntityMonster,
		ArchetypeID: "weapon-accuracy-target",
		BodySize:    world.EntityBodySizeSmall,
		Transform:   world.Transform{Position: world.Position{X: 2, Layer: 0}},
	}
	if err := sim.Spawn(monsterEntity, 4, .35, .5); err != nil {
		t.Fatal(err)
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:              "basic-attack",
		Targets:         []combat.TargetKind{combat.TargetEntity},
		Range:           4.5,
		BaseDamage:      100,
		DamageType:      combat.DamagePhysical,
		Blockable:       true,
		CooldownSeconds: .5,
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
		Entity: world.EntityState{
			ID:        10,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{Layer: 0}},
		},
		Speed: 6, Radius: .35, MaxStepHeight: .5,
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
	if err := inv.Add("item_light_guard_sword", 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotMainHand,
		ItemArchetypeID: "item_light_guard_sword",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip errors: %#v", report.CommandErrors)
	}

	attack := protocol.ClientUseAction{
		ActionID:   "basic-attack",
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "9101",
	}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("miss attack report=%#v", report)
	}
	event := waitForCombatEvent(t, conn)
	if event.ActorEntityID != s.EntityID || event.TargetEntityID != monsterID || event.Result != protocol.CombatEventMiss {
		t.Fatalf("miss event=%#v", event)
	}
	if event.Damage != 0 || event.Blocked {
		t.Fatalf("miss must not carry damage/block outcome: %#v", event)
	}
	if event.CooldownReadyTick != 20 {
		t.Fatalf("light guard sword cooldown ready tick=%d, want 20", event.CooldownReadyTick)
	}
	monster, ok := rt.combatantState(monsterID)
	if !ok || monster.HP != 200 {
		t.Fatalf("miss changed monster HP: %+v ok=%v", monster, ok)
	}

	if err := rt.EnqueueUseAction(s.ID, 3, attack); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(4, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 {
		t.Fatalf("immediate repeat after miss should be cooldown-rejected: %#v", report)
	}
	monster, ok = rt.combatantState(monsterID)
	if !ok || monster.HP != 200 {
		t.Fatalf("cooldown rejection after miss changed HP: %+v ok=%v", monster, ok)
	}
}
