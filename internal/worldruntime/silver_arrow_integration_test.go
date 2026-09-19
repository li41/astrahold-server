package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSilverArrowMaterialSurvivesExactAuthoritativeConsumption(t *testing.T) {
	runtime, s, _, _ := newBowAmmunitionRuntime(t, 6)
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add(ammunition.ItemSilverArrow, 1); err != nil {
		t.Fatal(err)
	}
	bow, ok := defaultEquipmentCatalog.Resolve("item_hunter_shortbow")
	if !ok {
		t.Fatal("hunter shortbow missing")
	}

	damage, material := runtime.rollEquippedBasicAttackWeaponDamageWithAmmunition(s.EntityID, s.ID, bow, equipmentcatalog.BodySizeSmall)
	if damage < 8 || damage > 11 {
		t.Fatalf("silver-arrow bow damage=%d want 8..11", damage)
	}
	if material != equipmentcatalog.MaterialSilver {
		t.Fatalf("fired ammunition material=%q want silver", material)
	}
	if inv.Quantity(ammunition.ItemSilverArrow) != 0 {
		t.Fatalf("silver arrow quantity=%d want 0", inv.Quantity(ammunition.ItemSilverArrow))
	}
}


func TestSilverArrowFeedsUndeadMultiplierAfterExactConsumption(t *testing.T) {
	runtime, s, _, _ := newBowAmmunitionRuntime(t, 6)
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add(ammunition.ItemSilverArrow, 1); err != nil {
		t.Fatal(err)
	}
	bow, ok := defaultEquipmentCatalog.Resolve("item_hunter_shortbow")
	if !ok {
		t.Fatal("hunter shortbow missing")
	}

	raw, firedMaterial := runtime.rollEquippedBasicAttackWeaponDamageWithAmmunition(
		s.EntityID,
		s.ID,
		bow,
		equipmentcatalog.BodySizeSmall,
	)
	if raw < 8 || raw > 11 {
		t.Fatalf("silver-arrow bow raw damage=%d want 8..11", raw)
	}
	if firedMaterial != equipmentcatalog.MaterialSilver {
		t.Fatalf("fired material=%q want silver", firedMaterial)
	}
	if inv.Quantity(ammunition.ItemSilverArrow) != 0 {
		t.Fatalf("silver arrow quantity=%d want 0", inv.Quantity(ammunition.ItemSilverArrow))
	}

	got := silverUndeadBasicAttackDamageWithAmmunitionV1(
		bow.Material,
		firedMaterial,
		world.EntityClassificationUndead,
		basicAttackActionID,
		combat.DamagePhysical,
		raw,
	)
	want := silverUndeadRawPhysicalDamageV1(raw)
	if got != want {
		t.Fatalf("silver-arrow undead damage=%d want=%d raw=%d", got, want, raw)
	}
}
