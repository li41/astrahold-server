package worldruntime

import (
	"testing"
	"time"

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

func TestEquippedBattleAxeDrivesAuthoritativeBasicAttackDamageAndTiming(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "low-tier-weapon-test",
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
		ID: monsterID, Kind: world.EntityMonster, ArchetypeID: "test-large-monster",
		Transform: world.Transform{Position: world.Position{X: 2, Z: 0, Layer: 0}},
	}
	if err := sim.Spawn(monsterEntity, 4, .35, .5); err != nil {
		t.Fatal(err)
	}

	combatCatalog, err := combat.NewService([]combat.ActionDefinition{{
		ID: "basic-attack", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
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
			HP: 200, MaxHP: 200, BodySize: equipmentcatalog.BodySizeLarge,
		},
		CorpseHoldTicks: 2, RespawnDelayTicks: 8,
	}
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(combatCatalog), WithMonsterLifecycle(lifecycle))
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
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}

	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add("item_militia_battle_axe", 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{
		Operation: protocol.EquipmentOperationEquip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemArchetypeID: "item_militia_battle_axe",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("equip errors: %#v", report.CommandErrors)
	}

	attack := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "9001"}
	if err := rt.EnqueueUseAction(s.ID, 2, attack); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first attack report=%#v", report)
	}
	firstEvent := waitForCombatEvent(t, conn)
	if firstEvent.ActorEntityID != s.EntityID || firstEvent.TargetEntityID != monsterID || firstEvent.Result != protocol.CombatEventHit {
		t.Fatalf("first event=%#v", firstEvent)
	}
	if firstEvent.Damage < 8 || firstEvent.Damage > 12 {
		t.Fatalf("large-target axe damage=%d, want 8..12", firstEvent.Damage)
	}
	if firstEvent.CooldownReadyTick != 26 {
		t.Fatalf("axe cooldown ready tick=%d, want 26", firstEvent.CooldownReadyTick)
	}
	monster, ok := rt.combatantState(monsterID)
	if !ok || monster.HP != 200-firstEvent.Damage {
		t.Fatalf("monster after first hit=%+v ok=%v event=%#v", monster, ok, firstEvent)
	}
	firstHP := monster.HP

	// Tick 14 is beyond the catalog's old 0.5-second action cooldown (10 ticks) but still
	// before the battle axe's authored 1.15-second interval (23 ticks from tick 3).
	if err := rt.EnqueueUseAction(s.ID, 3, attack); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(14, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 1 {
		t.Fatalf("early repeat report=%#v", report)
	}
	monster, ok = rt.combatantState(monsterID)
	if !ok || monster.HP != firstHP {
		t.Fatalf("cooldown-rejected attack changed HP: %+v ok=%v want=%d", monster, ok, firstHP)
	}

	if err := rt.EnqueueUseAction(s.ID, 4, attack); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(26, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("ready attack report=%#v", report)
	}
	secondEvent := waitForCombatEvent(t, conn)
	if secondEvent.Damage < 8 || secondEvent.Damage > 12 {
		t.Fatalf("second large-target axe damage=%d, want 8..12", secondEvent.Damage)
	}
	monster, ok = rt.combatantState(monsterID)
	if !ok || monster.HP != firstHP-secondEvent.Damage {
		t.Fatalf("monster after ready hit=%+v ok=%v second=%#v", monster, ok, secondEvent)
	}
}
