package worldruntime

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMonsterLootIsReservedForAuthoritativeKillerCharacter(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "loot-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
		BaseDamage: 200, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithMonsterLootCatalog(testMonsterLootCatalog(t)),
	)

	killerConn := session.NewQueueConnection(64, 16)
	thiefConn := session.NewQueueConnection(64, 16)
	killer, err := session.New(1, 10, 32, killerConn)
	if err != nil {
		t.Fatal(err)
	}
	thief, err := session.New(2, 20, 32, thiefConn)
	if err != nil {
		t.Fatal(err)
	}
	for _, join := range []JoinRequest{
		{
			Session: killer,
			Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}}},
			Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
		},
		{
			Session: thief,
			Entity: world.EntityState{ID: 20, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0.5, Layer: 0}}},
			Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
		},
	} {
		if err := rt.EnqueueJoin(join); err != nil {
			t.Fatal(err)
		}
	}
	spawn := testMonsterLootSpawn(testLootSourceArchetypeID)
	spawn.Entity.Transform.Position = world.Position{X: 1, Layer: 0}
	if err := rt.EnqueueSpawnEntity(spawn); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(1, 50*time.Millisecond))
	drainConnection(killerConn)
	drainConnection(thiefConn)

	if err := rt.EnqueueUseAction(killer.ID, 1, protocol.ClientUseAction{
		ActionID: "loot-strike", TargetKind: protocol.ActionTargetEntity,
		TargetID: strconv.FormatUint(uint64(spawn.Entity.ID), 10),
	}); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(2, 50*time.Millisecond))

	drops := itemDrops(sim)
	if len(drops) != 1 {
		t.Fatalf("drops=%#v", drops)
	}
	owner, restricted := rt.itemDropPickupOwner(drops[0].ID)
	if !restricted || owner != killer.CharacterIdentity.ID {
		t.Fatalf("owner=%q restricted=%v want killer=%q", owner, restricted, killer.CharacterIdentity.ID)
	}

	if err := rt.EnqueuePickupItem(thief.ID, 1, protocol.ClientPickupItem{DropEntityID: drops[0].ID}); err != nil {
		t.Fatal(err)
	}
	stolen := rt.Step(3, 50*time.Millisecond)
	if len(stolen.CommandErrors) != 1 || !errors.Is(stolen.CommandErrors[0].Err, ErrItemDropNotOwned) {
		t.Fatalf("unauthorized pickup=%#v", stolen)
	}
	if _, exists := sim.Entity(drops[0].ID); !exists {
		t.Fatal("unauthorized pickup removed authoritative drop")
	}
	if inv := rt.inventories[thief.CharacterIdentity.ID]; inv == nil || inv.Quantity(testLootItemArchetypeID) != 0 {
		t.Fatalf("thief inventory=%v", inv)
	}

	if err := rt.EnqueuePickupItem(killer.ID, 2, protocol.ClientPickupItem{DropEntityID: drops[0].ID}); err != nil {
		t.Fatal(err)
	}
	assertCleanMonsterLootStep(t, rt.Step(4, 50*time.Millisecond))
	if _, exists := sim.Entity(drops[0].ID); exists {
		t.Fatal("killer pickup left authoritative drop in world")
	}
	if _, restricted := rt.itemDropPickupOwner(drops[0].ID); restricted {
		t.Fatal("consumed drop retained pickup ownership")
	}
	if inv := rt.inventories[killer.CharacterIdentity.ID]; inv == nil || inv.Quantity(testLootItemArchetypeID) != 1 {
		t.Fatalf("killer inventory=%v", inv)
	}
}
