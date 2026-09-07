package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLowTierWeaponDamageRangesUseTargetSizeAndExtraDamage(t *testing.T) {
	axe, ok := defaultEquipmentCatalog.Resolve("item_militia_battle_axe")
	if !ok { t.Fatal("battle axe missing") }
	if got := rollWeaponDamage(axe, equipmentcatalog.BodySizeSmall, 0); got != 5 { t.Fatalf("axe small min = %d", got) }
	if got := rollWeaponDamage(axe, equipmentcatalog.BodySizeSmall, 3); got != 8 { t.Fatalf("axe small max = %d", got) }
	if got := rollWeaponDamage(axe, equipmentcatalog.BodySizeLarge, 0); got != 8 { t.Fatalf("axe large min = %d", got) }
	if got := rollWeaponDamage(axe, equipmentcatalog.BodySizeLarge, 4); got != 12 { t.Fatalf("axe large max = %d", got) }
	if got := rollWeaponDamage(axe, equipmentcatalog.BodySizeGiant, 4); got != 12 { t.Fatalf("axe giant compatibility = %d", got) }

	mace, ok := defaultEquipmentCatalog.Resolve("item_iron_war_mace")
	if !ok { t.Fatal("mace missing") }
	if got := rollWeaponDamage(mace, equipmentcatalog.BodySizeSmall, 0); got != 7 { t.Fatalf("mace min + extra = %d", got) }
	if got := rollWeaponDamage(mace, equipmentcatalog.BodySizeSmall, 3); got != 10 { t.Fatalf("mace max + extra = %d", got) }
}

func TestWeaponAttackIntervalsPreserveAuthoredTicksAt20Hz(t *testing.T) {
	cases := []struct {
		milliseconds uint32
		wantTicks    uint64
	}{
		{850, 17},
		{1000, 20},
		{1100, 22},
		{1150, 23},
	}
	for _, tc := range cases {
		definition := combat.ActionDefinition{CooldownSeconds: weaponAttackCooldownSeconds(tc.milliseconds)}
		got := combat.CooldownReadyTick(definition, 10, 50*time.Millisecond) - 10
		if got != tc.wantTicks {
			t.Fatalf("interval %dms = %d ticks, want %d", tc.milliseconds, got, tc.wantTicks)
		}
	}
}

func TestDirectSpawnRetainsAuthoritativeBodySizeAndClearsOnReuse(t *testing.T) {
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	rt := New(sim, DefaultConfig())
	request := SpawnEntityRequest{
		Entity: world.EntityState{ID: 9001, Kind: world.EntityMonster, ArchetypeID: "test-large-direct"},
		Speed: 4, Radius: .35, MaxStepHeight: .5, HP: 100, MaxHP: 100,
		BodySize: equipmentcatalog.BodySizeLarge,
	}
	if err := rt.EnqueueSpawnEntity(request); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("spawn errors = %#v", report.CommandErrors) }
	entity, ok := rt.world.Entity(9001)
	if !ok || entity.BodySize != world.EntityBodySizeLarge { t.Fatalf("spawned entity = %#v ok=%v", entity, ok) }
	if got := rt.entityWeaponBodySize(9001); got != equipmentcatalog.BodySizeLarge { t.Fatalf("body size = %q, want large", got) }

	// EntityState owns the classification, so normal world removal cannot leak a stale large size
	// when the same stable runtime EntityID is later reused by unclassified compatibility content.
	rt.world.Remove(9001)
	rt.characters.Remove(9001)
	request.Entity.ArchetypeID = "test-unclassified-direct"
	request.BodySize = ""
	if err := rt.EnqueueSpawnEntity(request); err != nil { t.Fatal(err) }
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("respawn errors = %#v", report.CommandErrors) }
	entity, ok = rt.world.Entity(9001)
	if !ok || entity.BodySize != "" { t.Fatalf("reused entity = %#v ok=%v", entity, ok) }
	if got := rt.entityWeaponBodySize(9001); got != equipmentcatalog.BodySizeSmall { t.Fatalf("fallback body size = %q, want small", got) }
}

func TestInventoryWeightsComeFromEquipmentCatalog(t *testing.T) {
	weights := defaultEquipmentCatalog.UnitWeights()
	if len(weights) != 8 { t.Fatalf("catalog weight count = %d, want 8", len(weights)) }
	for itemArchetypeID, want := range weights {
		inv := newCharacterInventory(32)
		if err := inv.Add(itemArchetypeID, 1); err != nil { t.Fatalf("add %s: %v", itemArchetypeID, err) }
		if got := inv.CurrentWeight(); got != uint64(want) {
			t.Fatalf("%s inventory weight = %d, catalog = %d", itemArchetypeID, got, want)
		}
	}
}

func TestRestoreCharacterInventoryPreservesMainAndOffHand(t *testing.T) {
	state, err := characterstate.NewInventoryStateWithEquipment(nil, "item_militia_iron_sword", "item_runed_square_shield")
	if err != nil { t.Fatal(err) }
	inv, err := restoreCharacterInventory(16, state)
	if err != nil { t.Fatal(err) }
	if inv.MainHand() != "item_militia_iron_sword" || inv.OffHand() != "item_runed_square_shield" {
		t.Fatalf("restored equipment = main:%q off:%q", inv.MainHand(), inv.OffHand())
	}
	if got := inv.CurrentWeight(); got != 13 { t.Fatalf("restored weight = %d", got) }
}

func TestRestoreCharacterInventoryRejectsWrongSlotKinds(t *testing.T) {
	state, err := characterstate.NewInventoryStateWithEquipment(nil, "item_guard_shield", "")
	if err != nil { t.Fatal(err) }
	if _, err := restoreCharacterInventory(16, state); err == nil { t.Fatal("shield restored into main hand") }

	state, err = characterstate.NewInventoryStateWithEquipment(nil, "", "item_militia_iron_sword")
	if err != nil { t.Fatal(err) }
	if _, err := restoreCharacterInventory(16, state); err == nil { t.Fatal("weapon restored into off hand") }
}
