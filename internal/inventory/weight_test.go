package inventory

import (
	"errors"
	"testing"
)

func TestCarryWeightRejectsAddAndRemoveFreesCapacity(t *testing.T) {
	inv := NewWithWeightPolicy(8, WeightPolicy{
		MaxWeight: 5, DefaultUnitWeight: 1,
		UnitWeights: map[string]uint32{"heavy": 3},
	})
	if err := inv.Add("heavy", 1); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != 3 || inv.MaxWeight() != 5 {
		t.Fatalf("weight=%d max=%d", inv.CurrentWeight(), inv.MaxWeight())
	}
	if err := inv.Add("heavy", 1); !errors.Is(err, ErrWeightExceeded) {
		t.Fatalf("second heavy add err=%v want weight exceeded", err)
	}
	if inv.Quantity("heavy") != 1 || inv.CurrentWeight() != 3 {
		t.Fatalf("rejected add mutated inventory quantity=%d weight=%d", inv.Quantity("heavy"), inv.CurrentWeight())
	}
	if err := inv.Remove("heavy", 1); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != 0 {
		t.Fatalf("remove left weight=%d", inv.CurrentWeight())
	}
	if err := inv.Add("heavy", 1); err != nil {
		t.Fatalf("capacity was not freed: %v", err)
	}
}

func TestExchangeChecksFinalCarryWeightAtomically(t *testing.T) {
	inv := NewWithWeightPolicy(8, WeightPolicy{
		MaxWeight: 5, DefaultUnitWeight: 1,
		UnitWeights: map[string]uint32{"coin": 1, "heavy": 5},
	})
	if err := inv.Add("coin", 2); err != nil {
		t.Fatal(err)
	}
	beforeRevision := inv.Revision()
	if err := inv.Exchange("coin", 1, "heavy", 1); !errors.Is(err, ErrWeightExceeded) {
		t.Fatalf("exchange err=%v want weight exceeded", err)
	}
	if inv.Quantity("coin") != 2 || inv.Quantity("heavy") != 0 || inv.CurrentWeight() != 2 || inv.Revision() != beforeRevision {
		t.Fatalf("rejected exchange mutated inventory coins=%d heavy=%d weight=%d revision=%d",
			inv.Quantity("coin"), inv.Quantity("heavy"), inv.CurrentWeight(), inv.Revision())
	}
}

func TestEquipmentPreservesCarryWeight(t *testing.T) {
	inv := NewWithWeightPolicy(8, WeightPolicy{
		MaxWeight: 10, DefaultUnitWeight: 1,
		UnitWeights: map[string]uint32{"blade": 8},
	})
	if err := inv.Add("blade", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipMainHand("blade"); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != 8 {
		t.Fatalf("equip changed carry weight=%d", inv.CurrentWeight())
	}
	if _, err := inv.UnequipMainHand(); err != nil {
		t.Fatal(err)
	}
	if inv.CurrentWeight() != 8 {
		t.Fatalf("unequip changed carry weight=%d", inv.CurrentWeight())
	}
}
