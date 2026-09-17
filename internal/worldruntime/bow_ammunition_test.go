package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestBowAndArrowDamageUsesOneCombinedUniformRange(t *testing.T) {
	bow := equipmentcatalog.Definition{
		ItemArchetypeID: "test_bow",
		Kind: equipmentcatalog.KindWeapon,
		Slot: equipmentcatalog.SlotMainHand,
		Tier: equipmentcatalog.TierLow,
		Weight: 1,
		Material: equipmentcatalog.MaterialWood,
		Weapon: &equipmentcatalog.Weapon{
			WeaponType: equipmentcatalog.WeaponTypeBow,
			SmallDamage: equipmentcatalog.DamageRange{Min: 2, Max: 3},
			LargeDamage: equipmentcatalog.DamageRange{Min: 2, Max: 3},
		},
	}
	arrow, ok := ammunition.Resolve(ammunition.ItemWoodArrow)
	if !ok {
		t.Fatal("wood arrow missing")
	}
	want := []uint32{8, 9, 10, 11, 8}
	for roll, expected := range want {
		if got := rollBowAndArrowDamage(bow, equipmentcatalog.BodySizeSmall, arrow, uint32(roll)); got != expected {
			t.Fatalf("roll %d damage=%d want %d", roll, got, expected)
		}
	}
}

func TestProductionBowTotalsRemainAuthoredWithEitherApprovedArrow(t *testing.T) {
	bows := []struct {
		bowID   string
		wantMin uint32
		wantMax uint32
	}{
		{"item_hunter_shortbow", 8, 11},
		{"item_mid_bow", 11, 15},
		{"item_high_bow", 14, 20},
	}
	for _, arrowID := range []string{ammunition.ItemWoodArrow, ammunition.ItemSilverArrow} {
		arrow, ok := ammunition.Resolve(arrowID)
		if !ok {
			t.Fatalf("arrow %q missing", arrowID)
		}
		for _, tc := range bows {
			bow, ok := defaultEquipmentCatalog.Resolve(tc.bowID)
			if !ok {
				t.Fatalf("bow %q missing", tc.bowID)
			}
			if got := rollBowAndArrowDamage(bow, equipmentcatalog.BodySizeSmall, arrow, 0); got != tc.wantMin {
				t.Fatalf("%s + %s minimum=%d want %d", tc.bowID, arrowID, got, tc.wantMin)
			}
			span := tc.wantMax - tc.wantMin + 1
			if got := rollBowAndArrowDamage(bow, equipmentcatalog.BodySizeSmall, arrow, span-1); got != tc.wantMax {
				t.Fatalf("%s + %s maximum=%d want %d", tc.bowID, arrowID, got, tc.wantMax)
			}
		}
	}
}

func TestCharacterInventoryTreatsApprovedArrowsAsZeroWeightStacks(t *testing.T) {
	inv := newCharacterInventory(8)
	for _, itemID := range []string{ammunition.ItemWoodArrow, ammunition.ItemSilverArrow} {
		if err := inv.Add(itemID, 1000); err != nil {
			t.Fatal(err)
		}
	}
	if inv.CurrentWeight() != 0 {
		t.Fatalf("approved arrows carry weight=%d want 0", inv.CurrentWeight())
	}
}
