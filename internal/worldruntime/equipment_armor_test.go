package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestLowTierArmorUsesAuthoritativeGenericSlotAndPersists(t *testing.T) {
	inv := newCharacterInventory(32)
	const helmet = "item_low_cloth_helmet"
	if err := inv.Add(helmet, 1); err != nil {
		t.Fatal(err)
	}
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotHelmet, helmet); err != nil {
		t.Fatalf("equip helmet: %v", err)
	}
	if got := inv.Equipped(inventory.SlotHelmet); got != helmet {
		t.Fatalf("helmet=%q want=%q", got, helmet)
	}

	durable, err := durableInventoryState(inv)
	if err != nil {
		t.Fatalf("durableInventoryState: %v", err)
	}
	restored, err := restoreCharacterInventory(32, durable)
	if err != nil {
		t.Fatalf("restoreCharacterInventory: %v", err)
	}
	if got := restored.Equipped(inventory.SlotHelmet); got != helmet {
		t.Fatalf("restored helmet=%q want=%q", got, helmet)
	}
}

func TestLowTierArmorRejectsWrongSlot(t *testing.T) {
	inv := newCharacterInventory(32)
	const helmet = "item_low_heavy_helmet"
	if err := inv.Add(helmet, 1); err != nil {
		t.Fatal(err)
	}
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotChest, helmet); err != ErrEquipmentItemNotAllowed {
		t.Fatalf("wrong-slot equip err=%v want=%v", err, ErrEquipmentItemNotAllowed)
	}
	if got := inv.Quantity(helmet); got != 1 {
		t.Fatalf("wrong-slot rejection mutated inventory quantity=%d", got)
	}
	if got := inv.Equipped(inventory.SlotChest); got != "" {
		t.Fatalf("wrong-slot rejection equipped chest=%q", got)
	}
}
