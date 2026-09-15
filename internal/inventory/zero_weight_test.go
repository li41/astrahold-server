package inventory

import "testing"

func TestExplicitZeroWeightOverrideDoesNotConsumeCarryCapacity(t *testing.T) {
	inv := NewWithWeightPolicy(8, WeightPolicy{
		MaxWeight: 5, DefaultUnitWeight: 1,
		UnitWeights: map[string]uint32{"arrow": 0, "stone": 1},
	})
	if err := inv.Add("arrow", 1000); err != nil {
		t.Fatalf("add zero-weight arrows: %v", err)
	}
	if inv.CurrentWeight() != 0 {
		t.Fatalf("zero-weight arrows changed carry weight=%d", inv.CurrentWeight())
	}
	if err := inv.Add("stone", 5); err != nil {
		t.Fatalf("add weighted items after arrows: %v", err)
	}
	if inv.CurrentWeight() != 5 {
		t.Fatalf("weighted items carry weight=%d want 5", inv.CurrentWeight())
	}
	if err := inv.Remove("arrow", 1); err != nil {
		t.Fatalf("remove arrow: %v", err)
	}
	if inv.CurrentWeight() != 5 {
		t.Fatalf("removing zero-weight arrow changed carry weight=%d", inv.CurrentWeight())
	}
}
