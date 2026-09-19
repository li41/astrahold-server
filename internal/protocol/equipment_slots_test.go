package protocol

import (
	"reflect"
	"testing"
)

func TestProtocolV31EquipmentSlots(t *testing.T) {
	if Version < 31 {
		t.Fatalf("protocol version = %d, want >=31", Version)
	}
	want := []EquipmentSlot{
		EquipmentSlotMainHand,
		EquipmentSlotOffHand,
		EquipmentSlotHelmet,
		EquipmentSlotChest,
		EquipmentSlotGloves,
		EquipmentSlotLegs,
		EquipmentSlotBoots,
		EquipmentSlotNecklace,
		EquipmentSlotRing1,
		EquipmentSlotRing2,
		EquipmentSlotBelt,
	}
	if got := EquipmentSlots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("equipment slots = %#v, want %#v", got, want)
	}
}
