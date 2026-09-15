package inventory

import (
	"reflect"
	"testing"
)

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
	}
	for _, item := range items {
		if err := inv.Add(item.id, 1); err != nil { t.Fatalf("Add(%s): %v", item.id, err) }
		if err := inv.Equip(item.slot, item.id); err != nil { t.Fatalf("Equip(%s): %v", item.slot, err) }
		if got := inv.Equipped(item.slot); got != item.id { t.Fatalf("Equipped(%s)=%q want %q", item.slot, got, item.id) }
	}
	want := []EquippedArchetype{
		{Slot: SlotHelmet, ArchetypeID: "helmet"},
		{Slot: SlotChest, ArchetypeID: "chest"},
		{Slot: SlotGloves, ArchetypeID: "gloves"},
		{Slot: SlotLegs, ArchetypeID: "legs"},
		{Slot: SlotBoots, ArchetypeID: "boots"},
	}
	if got := inv.EquippedArchetypeSnapshot(); !reflect.DeepEqual(got, want) { t.Fatalf("snapshot=%#v want %#v", got, want) }
	if got, err := inv.Unequip(SlotChest); err != nil || got != "chest" { t.Fatalf("Unequip(chest)=(%q,%v)", got, err) }
	if inv.Quantity("chest") != 1 { t.Fatalf("chest quantity=%d want 1", inv.Quantity("chest")) }
}

func TestHandWrappersUseGenericEquipmentSlots(t *testing.T) {
	inv := New(4)
	if err := inv.Add("blade", 1); err != nil { t.Fatal(err) }
	if err := inv.Add("shield", 1); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHand("blade"); err != nil { t.Fatal(err) }
	if err := inv.EquipOffHand("shield"); err != nil { t.Fatal(err) }
	if inv.Equipped(SlotMainHand) != "blade" || inv.MainHand() != "blade" { t.Fatalf("main hand wrapper diverged") }
	if inv.Equipped(SlotOffHand) != "shield" || inv.OffHand() != "shield" { t.Fatalf("off hand wrapper diverged") }
}
