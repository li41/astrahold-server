package inventory

import (
	"testing"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestRemoveOwnedInstanceUnequipped(t *testing.T) {
	inv := NewWithWeightPolicy(4, WeightPolicy{DefaultUnitWeight: 5})
	instance := iteminstance.Instance{ID: "instance:blade", ItemArchetypeID: "item_blade", EnhancementLevel: 12}
	if err := inv.AddInstance(instance); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}
	beforeRevision := inv.Revision()
	beforeEquipmentRevision := inv.EquipmentRevision()

	removed, equipped, err := inv.RemoveOwnedInstance(instance.ID)
	if err != nil {
		t.Fatalf("RemoveOwnedInstance: %v", err)
	}
	if equipped {
		t.Fatal("unequipped instance reported as equipped")
	}
	if removed.ID != instance.ID || removed.EnhancementLevel != 12 {
		t.Fatalf("removed=%+v", removed)
	}
	if _, _, ok := inv.OwnedInstance(instance.ID); ok {
		t.Fatal("destroyed instance still owned")
	}
	if got := inv.CurrentWeight(); got != 0 {
		t.Fatalf("CurrentWeight=%d want=0", got)
	}
	if got := inv.Revision(); got != beforeRevision+1 {
		t.Fatalf("Revision=%d want=%d", got, beforeRevision+1)
	}
	if got := inv.EquipmentRevision(); got != beforeEquipmentRevision {
		t.Fatalf("EquipmentRevision=%d want=%d", got, beforeEquipmentRevision)
	}
}

func TestRemoveOwnedInstanceEquippedClearsSlotAndAdvancesEquipmentRevision(t *testing.T) {
	inv := NewWithWeightPolicy(4, WeightPolicy{DefaultUnitWeight: 5})
	instance := iteminstance.Instance{ID: "instance:blade", ItemArchetypeID: "item_blade", EnhancementLevel: 12}
	if err := inv.AddInstance(instance); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}
	if err := inv.EquipInstance(SlotMainHand, instance.ID); err != nil {
		t.Fatalf("EquipInstance: %v", err)
	}
	beforeRevision := inv.Revision()
	beforeEquipmentRevision := inv.EquipmentRevision()

	removed, equipped, err := inv.RemoveOwnedInstance(instance.ID)
	if err != nil {
		t.Fatalf("RemoveOwnedInstance: %v", err)
	}
	if !equipped {
		t.Fatal("equipped instance was not reported as equipped")
	}
	if removed.ID != instance.ID {
		t.Fatalf("removed=%+v", removed)
	}
	if _, ok := inv.EquippedInstance(SlotMainHand); ok {
		t.Fatal("destroyed instance remained equipped")
	}
	if got := inv.Equipped(SlotMainHand); got != "" {
		t.Fatalf("Equipped(main_hand)=%q want empty", got)
	}
	if got := inv.CurrentWeight(); got != 0 {
		t.Fatalf("CurrentWeight=%d want=0", got)
	}
	if got := inv.Revision(); got != beforeRevision+1 {
		t.Fatalf("Revision=%d want=%d", got, beforeRevision+1)
	}
	if got := inv.EquipmentRevision(); got != beforeEquipmentRevision+1 {
		t.Fatalf("EquipmentRevision=%d want=%d", got, beforeEquipmentRevision+1)
	}
}
