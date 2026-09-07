package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
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
