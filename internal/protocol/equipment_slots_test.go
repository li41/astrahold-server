package protocol

import (
	"reflect"
	"testing"
)

func TestProtocolV29EquipmentSlots(t *testing.T) {
	if Version != 29 {
		t.Fatalf("protocol version = %d, want 29", Version)
	}
	want := []EquipmentSlot{
		EquipmentSlotMainHand,
		EquipmentSlotOffHand,
		EquipmentSlotHelmet,
		EquipmentSlotChest,
		EquipmentSlotGloves,
		EquipmentSlotLegs,
		EquipmentSlotBoots,
	}
	if got := EquipmentSlots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("equipment slots = %#v, want %#v", got, want)
	}
}
