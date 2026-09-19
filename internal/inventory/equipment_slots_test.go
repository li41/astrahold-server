package inventory

import (
	"reflect"
	"testing"
)

func TestEquipmentSlotsAreStableAndDistinct(t *testing.T) {
	want := []EquipmentSlot{
		SlotMainHand,
		SlotOffHand,
		SlotHelmet,
		SlotChest,
		SlotGloves,
		SlotLegs,
		SlotBoots,
		SlotNecklace,
		SlotRing1,
		SlotRing2,
		SlotBelt,
	}
	if got := EquipmentSlots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("EquipmentSlots()=%#v want %#v", got, want)
	}
	for _, slot := range want {
		if !ValidEquipmentSlot(slot) {
			t.Fatalf("slot %q rejected", slot)
		}
	}
	if SlotRing1 == SlotRing2 {
		t.Fatal("ring_1 and ring_2 must be distinct authoritative locations")
	}
}

func TestGenericEquipmentSlotsEquipAndUnequip(t *testing.T) {
	inv := New(32)
	items := []struct {
		slot EquipmentSlot
		id   string
	}{
		{SlotHelmet, "helmet"},
		{SlotChest, "chest"},
		{SlotGloves, "gloves"},
		{SlotLegs, "legs"},
		{SlotBoots, "boots"},
		{SlotNecklace, "necklace"},
		{SlotRing1, "ring-a"},
		{SlotRing2, "ring-b"},
		{SlotBelt, "belt"},
	}
	for _, item := range items {
		if err := inv.Add(item.id, 1); err != nil {
			t.Fatalf("Add(%s): %v", item.id, err)
		}
		if err := inv.Equip(item.slot, item.id); err != nil {
			t.Fatalf("Equip(%s): %v", item.slot, err)
		}
		if got := inv.Equipped(item.slot); got != item.id {
			t.Fatalf("Equipped(%s)=%q want %q", item.slot, got, item.id)
		}
	}
	want := []EquippedArchetype{
		{Slot: SlotHelmet, ArchetypeID: "helmet"},
		{Slot: SlotChest, ArchetypeID: "chest"},
		{Slot: SlotGloves, ArchetypeID: "gloves"},
		{Slot: SlotLegs, ArchetypeID: "legs"},
		{Slot: SlotBoots, ArchetypeID: "boots"},
		{Slot: SlotNecklace, ArchetypeID: "necklace"},
		{Slot: SlotRing1, ArchetypeID: "ring-a"},
		{Slot: SlotRing2, ArchetypeID: "ring-b"},
		{Slot: SlotBelt, ArchetypeID: "belt"},
	}
	if got := inv.EquippedArchetypeSnapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot=%#v want %#v", got, want)
	}
	if got, err := inv.Unequip(SlotChest); err != nil || got != "chest" {
		t.Fatalf("Unequip(chest)=(%q,%v)", got, err)
	}
	if inv.Quantity("chest") != 1 {
		t.Fatalf("chest quantity=%d want 1", inv.Quantity("chest"))
	}
}

func TestHandWrappersUseGenericEquipmentSlots(t *testing.T) {
	inv := New(4)
	if err := inv.Add("blade", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.Add("shield", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipMainHand("blade"); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipOffHand("shield"); err != nil {
		t.Fatal(err)
	}
	if inv.Equipped(SlotMainHand) != "blade" || inv.MainHand() != "blade" {
		t.Fatalf("main hand wrapper diverged")
	}
	if inv.Equipped(SlotOffHand) != "shield" || inv.OffHand() != "shield" {
		t.Fatalf("off hand wrapper diverged")
	}
}
