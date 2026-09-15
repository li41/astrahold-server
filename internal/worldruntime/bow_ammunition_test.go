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
	arrow, ok := ammunition.Resolve(ammunition.ItemLowArrow)
	if !ok {
		t.Fatal("low arrow missing")
	}
	want := []uint32{8, 9, 10, 11, 8}
	for roll, expected := range want {
		if got := rollBowAndArrowDamage(bow, equipmentcatalog.BodySizeSmall, arrow, uint32(roll)); got != expected {
			t.Fatalf("roll %d damage=%d want %d", roll, got, expected)
		}
	}
}

func TestCharacterInventoryTreatsV1ArrowsAsZeroWeightStacks(t *testing.T) {
	inv := newCharacterInventory(8)
	if err := inv.Add(ammunition.ItemLowArrow, 1000); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != 0 {
		t.Fatalf("1000 arrows carry weight=%d want 0", inv.CurrentWeight())
	}
}
