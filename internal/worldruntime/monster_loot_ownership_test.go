package worldruntime

import (
	"strconv"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMonsterLootSingleNearbyContributorAutoAddsToInventory(t *testing.T) {
	rt, sim := newLootDistributionRuntime(t, []combat.ActionDefinition{{
		ID: "loot-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6,
		BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	player, conn := addLootDistributionPlayer(t, rt, 1, 10, "loot-single", 0)
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(conn)

	enqueueLootAttack(t, rt, player.ID, 1, "loot-strike", spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))

	if drops := itemDrops(sim); len(drops) != 0 {
		t.Fatalf("nearby single contributor should auto-loot, drops=%#v", drops)
	}
	inv := rt.inventories[player.CharacterIdentity.ID]
	if inv == nil || inv.Quantity(testLootItemArchetypeID) != 1 {
		t.Fatalf("inventory=%v pelt=%d", inv, inventoryQuantity(inv, testLootItemArchetypeID))
	}
	state := rt.monsterLootStates[spawn.Entity.ID]
	if state == nil || state.contributions[player.CharacterIdentity.ID].damage != 200 {
		t.Fatalf("damage contribution=%#v", state)
	}
}

func TestMonsterLootSingleNearbyContributorOverweightLeavesPublicGroundDrop(t *testing.T) {
	rt, sim := newLootDistributionRuntime(t, []combat.ActionDefinition{{
		ID: "loot-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6,
		BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	killer, killerConn := addLootDistributionPlayer(t, rt, 1, 10, "loot-heavy", 0)
	scavenger, scavengerConn := addLootDistributionPlayer(t, rt, 2, 20, "loot-scavenger", 0.5)
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(killerConn)
	drainConnection(scavengerConn)

	// The pelt weighs two units in this focused policy, while the killer can carry only one.
	rt.inventories[killer.CharacterIdentity.ID] = inventory.NewWithWeightPolicy(32, inventory.WeightPolicy{
		MaxWeight: 1, DefaultUnitWeight: 1,
		UnitWeights: map[string]uint32{testLootItemArchetypeID: 2},
	})

	enqueueLootAttack(t, rt, killer.ID, 1, "loot-strike", spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	drops := itemDrops(sim)
	if len(drops) != 1 {
		t.Fatalf("overweight auto-loot must remain ground drop, drops=%#v", drops)
	}
	if inventoryQuantity(rt.inventories[killer.CharacterIdentity.ID], testLootItemArchetypeID) != 0 {
		t.Fatal("overweight killer received loot")
	}

	// Ground loot has no reservation window: a nearby non-contributor can take it immediately.
	if err := rt.EnqueuePickupItem(scavenger.ID, 1, protocol.ClientPickupItem{DropEntityID: drops[0].ID}); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(3, 50*time.Millisecond))
	if _, exists := sim.Entity(drops[0].ID); exists {
		t.Fatal("public ground drop still exists after scavenger pickup")
	}
	if inventoryQuantity(rt.inventories[scavenger.CharacterIdentity.ID], testLootItemArchetypeID) != 1 {
		t.Fatal("scavenger did not receive public ground loot")
	}
}

func TestMonsterLootContributorOutsideAutoLootRadiusLeavesPublicGroundDrop(t *testing.T) {
	rt, sim := newLootDistributionRuntime(t, []combat.ActionDefinition{{
		ID: "ranged-loot-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6,
		BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	shooter, shooterConn := addLootDistributionPlayer(t, rt, 1, 10, "loot-ranged", 5)
	scavenger, scavengerConn := addLootDistributionPlayer(t, rt, 2, 20, "loot-nearby", 0.5)
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(shooterConn)
	drainConnection(scavengerConn)

	enqueueLootAttack(t, rt, shooter.ID, 1, "ranged-loot-strike", spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	drops := itemDrops(sim)
	if len(drops) != 1 {
		t.Fatalf("out-of-radius contributor must leave public ground loot, drops=%#v", drops)
	}
	if inventoryQuantity(rt.inventories[shooter.CharacterIdentity.ID], testLootItemArchetypeID) != 0 {
		t.Fatal("out-of-radius contributor received auto-loot")
	}

	if err := rt.EnqueuePickupItem(scavenger.ID, 1, protocol.ClientPickupItem{DropEntityID: drops[0].ID}); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(3, 50*time.Millisecond))
	if inventoryQuantity(rt.inventories[scavenger.CharacterIdentity.ID], testLootItemArchetypeID) != 1 {
		t.Fatal("nearby scavenger could not pick public drop")
	}
}

func TestMonsterLootMultipleNearbyContributorsAwardsExactlyOneWeightedWinner(t *testing.T) {
	rt, sim := newLootDistributionRuntime(t, []combat.ActionDefinition{
		{ID: "heavy-hit", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6, BaseDamage: 80, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5},
		{ID: "light-hit", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6, BaseDamage: 20, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5},
	})
	heavy, heavyConn := addLootDistributionPlayer(t, rt, 1, 10, "loot-heavy-damage", 0)
	light, lightConn := addLootDistributionPlayer(t, rt, 2, 20, "loot-light-damage", 0.5)
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.HP = 100
	spawn.MaxHP = 100
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(heavyConn)
	drainConnection(lightConn)

	enqueueLootAttack(t, rt, heavy.ID, 1, "heavy-hit", spawn.Entity.ID)
	enqueueLootAttack(t, rt, light.ID, 1, "light-hit", spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))

	if drops := itemDrops(sim); len(drops) != 0 {
		t.Fatalf("weighted winner had capacity, unexpected ground drops=%#v", drops)
	}
	heavyQty := inventoryQuantity(rt.inventories[heavy.CharacterIdentity.ID], testLootItemArchetypeID)
	lightQty := inventoryQuantity(rt.inventories[light.CharacterIdentity.ID], testLootItemArchetypeID)
	if heavyQty+lightQty != 1 {
		t.Fatalf("weighted distribution granted %d items; heavy=%d light=%d", heavyQty+lightQty, heavyQty, lightQty)
	}
	state := rt.monsterLootStates[spawn.Entity.ID]
	if state == nil || state.contributions[heavy.CharacterIdentity.ID].damage != 80 || state.contributions[light.CharacterIdentity.ID].damage != 20 {
		t.Fatalf("contributions=%#v", state)
	}
}

func TestDamageWeightedMonsterLootCandidateUsesExactDamageShare(t *testing.T) {
	candidates := []monsterLootCandidate{
		{characterID: "a", damage: 80},
		{characterID: "b", damage: 20},
	}
	for ticket := uint64(0); ticket < 100; ticket++ {
		winner, ok := selectDamageWeightedMonsterLootCandidate(candidates, ticket)
		if !ok {
			t.Fatalf("ticket=%d produced no winner", ticket)
		}
		want := characteridentity.ID("a")
		if ticket >= 80 {
			want = "b"
		}
		if winner.characterID != want {
			t.Fatalf("ticket=%d winner=%q want=%q", ticket, winner.characterID, want)
		}
	}
}

func TestMonsterLootDamageContributionCanBeClearedForEvadeReset(t *testing.T) {
	rt, _ := newLootDistributionRuntime(t, []combat.ActionDefinition{{
		ID: "loot-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 6,
		BaseDamage: 50, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	player, conn := addLootDistributionPlayer(t, rt, 1, 10, "loot-reset", 0)
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(conn)

	enqueueLootAttack(t, rt, player.ID, 1, "loot-strike", spawn.Entity.ID)
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))
	state := rt.monsterLootStates[spawn.Entity.ID]
	if state == nil || state.contributions[player.CharacterIdentity.ID].damage != 50 {
		t.Fatalf("contribution before reset=%#v", state)
	}
	rt.resetMonsterLootContributions(spawn.Entity.ID)
	if len(state.contributions) != 0 {
		t.Fatalf("contributions survived encounter reset: %#v", state.contributions)
	}
}

func newLootDistributionRuntime(t *testing.T, actions []combat.ActionDefinition) (*Runtime, *simulation.World) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	combatService, err := combat.NewService(actions)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	return New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithMonsterLootCatalog(testMonsterLootCatalog(t)),
	), sim
}

func addLootDistributionPlayer(t *testing.T, rt *Runtime, sessionID session.ID, entityID world.EntityID, characterID string, x float32) (*session.Session, *session.QueueConnection) {
	t.Helper()
	identity, err := characteridentity.NewTrusted(characterID)
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(128, 32)
	s, err := session.NewWithCharacterIdentity(sessionID, entityID, identity, 32, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: entityID, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: x, Layer: 0}}},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	return s, conn
}

func enqueueLootAttack(t *testing.T, rt *Runtime, sessionID session.ID, sequence uint32, actionID string, monsterID world.EntityID) {
	t.Helper()
	if err := rt.EnqueueUseAction(sessionID, sequence, protocol.ClientUseAction{
		ActionID: actionID, TargetKind: protocol.ActionTargetEntity,
		TargetID: strconv.FormatUint(uint64(monsterID), 10),
	}); err != nil {
		t.Fatal(err)
	}
}

func inventoryQuantity(inv *inventory.Inventory, archetypeID string) uint32 {
	if inv == nil {
		return 0
	}
	return inv.Quantity(archetypeID)
}
