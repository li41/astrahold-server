package inventory

import (
	"errors"
	"testing"
)

func TestEquipAndUnequipMainHandMovesOneItemAtomically(t *testing.T) {
	inv := New(4)
	if err := inv.Add("item_training_blade", 1); err != nil {
		t.Fatal(err)
	}
	if got := inv.Revision(); got != 1 {
		t.Fatalf("starter revision=%d want=1", got)
	}

	if err := inv.EquipMainHand("item_training_blade"); err != nil {
		t.Fatal(err)
	}
	if got := inv.Quantity("item_training_blade"); got != 0 {
		t.Fatalf("inventory blade quantity=%d want=0", got)
	}
	if got := inv.MainHand(); got != "item_training_blade" {
		t.Fatalf("main hand=%q", got)
	}
	if got := inv.Revision(); got != 2 {
		t.Fatalf("inventory revision=%d want=2", got)
	}
	if got := inv.EquipmentRevision(); got != 1 {
		t.Fatalf("equipment revision=%d want=1", got)
	}

	item, err := inv.UnequipMainHand()
	if err != nil {
		t.Fatal(err)
	}
	if item != "item_training_blade" {
		t.Fatalf("unequipped=%q", item)
	}
	if got := inv.Quantity("item_training_blade"); got != 1 {
		t.Fatalf("restored blade quantity=%d want=1", got)
	}
	if got := inv.MainHand(); got != "" {
		t.Fatalf("main hand=%q want empty", got)
	}
	if got := inv.Revision(); got != 3 {
		t.Fatalf("inventory revision=%d want=3", got)
	}
	if got := inv.EquipmentRevision(); got != 2 {
		t.Fatalf("equipment revision=%d want=2", got)
	}
}

func TestEquipMainHandRejectionDoesNotMutateState(t *testing.T) {
	inv := New(4)
	if err := inv.Add("item_training_blade", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipMainHand("item_training_blade"); err != nil {
		t.Fatal(err)
	}
	inventoryRevision := inv.Revision()
	equipmentRevision := inv.EquipmentRevision()

	if err := inv.EquipMainHand("item_other_blade"); !errors.Is(err, ErrEquipmentSlotOccupied) {
		t.Fatalf("err=%v want occupied", err)
	}
	if inv.Revision() != inventoryRevision || inv.EquipmentRevision() != equipmentRevision || inv.MainHand() != "item_training_blade" {
		t.Fatal("rejected equip mutated authoritative state")
	}
}

func TestUnequipMainHandFullInventoryDoesNotLoseEquipment(t *testing.T) {
	inv := New(1)
	if err := inv.Add("item_training_blade", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipMainHand("item_training_blade"); err != nil {
		t.Fatal(err)
	}
	if err := inv.Add("item_minor_healing_potion", 1); err != nil {
		t.Fatal(err)
	}
	inventoryRevision := inv.Revision()
	equipmentRevision := inv.EquipmentRevision()

	if _, err := inv.UnequipMainHand(); !errors.Is(err, ErrFull) {
		t.Fatalf("err=%v want full", err)
	}
	if inv.Revision() != inventoryRevision || inv.EquipmentRevision() != equipmentRevision {
		t.Fatal("rejected unequip changed revisions")
	}
	if got := inv.MainHand(); got != "item_training_blade" {
		t.Fatalf("equipped item lost: %q", got)
	}
	if got := inv.Quantity("item_minor_healing_potion"); got != 1 {
		t.Fatalf("inventory changed unexpectedly: %d", got)
	}
}
