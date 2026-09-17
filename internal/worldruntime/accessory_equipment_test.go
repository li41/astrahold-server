package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestAccessoryEquipmentSlotMappings(t *testing.T) {
	cases := []struct {
		wire    protocol.EquipmentSlot
		storage inventory.EquipmentSlot
		catalog equipmentcatalog.Slot
	}{
		{protocol.EquipmentSlotNecklace, inventory.SlotNecklace, equipmentcatalog.SlotNecklace},
		{protocol.EquipmentSlotRing1, inventory.SlotRing1, equipmentcatalog.SlotRing},
		{protocol.EquipmentSlotRing2, inventory.SlotRing2, equipmentcatalog.SlotRing},
		{protocol.EquipmentSlotBelt, inventory.SlotBelt, equipmentcatalog.SlotBelt},
	}
	for _, tc := range cases {
		storage, ok := inventoryEquipmentSlot(tc.wire)
		if !ok || storage != tc.storage {
			t.Fatalf("inventoryEquipmentSlot(%q)=%q ok=%v want=%q", tc.wire, storage, ok, tc.storage)
		}
		wire, ok := protocolEquipmentSlot(tc.storage)
		if !ok || wire != tc.wire {
			t.Fatalf("protocolEquipmentSlot(%q)=%q ok=%v want=%q", tc.storage, wire, ok, tc.wire)
		}
		catalog, ok := catalogEquipmentSlot(tc.storage)
		if !ok || catalog != tc.catalog {
			t.Fatalf("catalogEquipmentSlot(%q)=%q ok=%v want=%q", tc.storage, catalog, ok, tc.catalog)
		}
		kind, ok := expectedEquipmentKind(tc.storage)
		if !ok || kind != equipmentcatalog.KindAccessory {
			t.Fatalf("expectedEquipmentKind(%q)=%q ok=%v want=%q", tc.storage, kind, ok, equipmentcatalog.KindAccessory)
		}
	}
}

func TestAccessoryCannotUseEnhancementScrolls(t *testing.T) {
	if enhancementScrollAllows(ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindAccessory) {
		t.Fatal("armor enhancement scroll accepted accessory")
	}
	if enhancementScrollAllows(WeaponEnhancementScrollItemArchetypeID, equipmentcatalog.KindAccessory) {
		t.Fatal("weapon enhancement scroll accepted accessory")
	}
	if _, ok := equipmentEnhancementChanceFor(equipmentcatalog.KindAccessory, 0); ok {
		t.Fatal("accessory unexpectedly has enhancement probability table")
	}
}
