package inventory

import (
	"errors"
	"testing"
)

func TestOffHandEquipmentIsIndependentAndKeepsCarryWeight(t *testing.T) {
	inv := NewWithWeightPolicy(8, WeightPolicy{MaxWeight: 100, DefaultUnitWeight: 1, UnitWeights: map[string]uint32{"sword": 7, "shield": 6}})
	if err := inv.Add("sword", 1); err != nil { t.Fatal(err) }
	if err := inv.Add("shield", 1); err != nil { t.Fatal(err) }
	if got := inv.CurrentWeight(); got != 13 { t.Fatalf("weight before equip = %d", got) }

	if err := inv.EquipMainHand("sword"); err != nil { t.Fatal(err) }
	if err := inv.EquipOffHand("shield"); err != nil { t.Fatal(err) }
	if inv.MainHand() != "sword" || inv.OffHand() != "shield" { t.Fatalf("equipment = main:%q off:%q", inv.MainHand(), inv.OffHand()) }
	if got := inv.CurrentWeight(); got != 13 { t.Fatalf("weight after equip = %d", got) }
	if got := inv.EquipmentRevision(); got != 2 { t.Fatalf("equipment revision = %d", got) }

	if _, err := inv.UnequipOffHand(); err != nil { t.Fatal(err) }
	if inv.MainHand() != "sword" || inv.OffHand() != "" { t.Fatalf("equipment after offhand unequip = main:%q off:%q", inv.MainHand(), inv.OffHand()) }
	if got := inv.Quantity("shield"); got != 1 { t.Fatalf("shield inventory = %d", got) }
	if got := inv.CurrentWeight(); got != 13 { t.Fatalf("weight after unequip = %d", got) }
}

func TestNilInventoryEquipmentMethodsDoNotPanic(t *testing.T) {
	var inv *Inventory
	if err := inv.EquipMainHand("sword"); !errors.Is(err, ErrInsufficient) { t.Fatalf("main equip err = %v", err) }
	if err := inv.EquipOffHand("shield"); !errors.Is(err, ErrInsufficient) { t.Fatalf("off equip err = %v", err) }
	if _, err := inv.UnequipMainHand(); !errors.Is(err, ErrEquipmentSlotEmpty) { t.Fatalf("main unequip err = %v", err) }
	if _, err := inv.UnequipOffHand(); !errors.Is(err, ErrEquipmentSlotEmpty) { t.Fatalf("off unequip err = %v", err) }
}
