package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const testSilverUndeadWeapon = "item_test_silver_undead_weapon"

func withSilverUndeadIntegrationCatalog(t *testing.T) {
	t.Helper()
	catalog, err := equipmentcatalog.New(equipmentcatalog.CatalogDefinition{
		Revision: "silver-undead-integration-test",
		WeaponTypes: []equipmentcatalog.WeaponTypeDefinition{{
			WeaponType: equipmentcatalog.WeaponTypeDagger,
		}},
		Items: []equipmentcatalog.Definition{{
			ItemArchetypeID: testSilverUndeadWeapon,
			Kind:            equipmentcatalog.KindWeapon,
			Slot:            equipmentcatalog.SlotMainHand,
			Tier:            equipmentcatalog.TierLow,
			Weight:          1,
			Material:        equipmentcatalog.MaterialSilver,
			Weapon: &equipmentcatalog.Weapon{
				WeaponType:  equipmentcatalog.WeaponTypeDagger,
				SmallDamage: equipmentcatalog.DamageRange{Min: 10, Max: 10},
				LargeDamage: equipmentcatalog.DamageRange{Min: 10, Max: 10},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldCatalog := defaultEquipmentCatalog
	defaultEquipmentCatalog = catalog
	t.Cleanup(func() { defaultEquipmentCatalog = oldCatalog })
}

func TestEquippedSilverMainHandBasicAttackGetsUndeadBonusFromAuthoritativeTargetState(t *testing.T) {
	withSilverUndeadIntegrationCatalog(t)

	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	undeadID := world.EntityID(901)
	nonUndeadID := world.EntityID(902)
	for _, entity := range []world.EntityState{
		{ID: undeadID, Kind: world.EntityMonster, ArchetypeID: "monster-test-undead", Classification: world.EntityClassificationUndead},
		{ID: nonUndeadID, Kind: world.EntityMonster, ArchetypeID: "monster-test-living"},
	} {
		if err := sim.Spawn(entity, 4, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(51, 510, 52, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session:       s,
		Entity:        world.EntityState{ID: 510, Kind: world.EntityPlayer},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}

	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add(testSilverUndeadWeapon, 1); err != nil {
		t.Fatal(err)
	}
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotMainHand, testSilverUndeadWeapon); err != nil {
		t.Fatal(err)
	}

	prepared := combat.PreparedAction{
		Definition: combat.ActionDefinition{ID: basicAttackActionID, Effect: combat.EffectDamage},
		Target:     combat.Target{Kind: combat.TargetEntity},
		Damage:     combat.Damage{Type: combat.DamagePhysical, Amount: 999},
	}

	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, undeadID, prepared); got != 12 {
		t.Fatalf("silver basic attack vs undead=%d want=12", got)
	}
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, nonUndeadID, prepared); got != 10 {
		t.Fatalf("silver basic attack vs non-undead=%d want=10", got)
	}
}
