package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestEquipmentDefinitionLegalityUsesKindAndSlotOnly(t *testing.T) {
	definition := equipmentcatalog.Definition{
		ItemArchetypeID: "item_test_weapon",
		Kind:            equipmentcatalog.KindWeapon,
		Slot:            equipmentcatalog.SlotMainHand,
	}
	if !equipmentDefinitionAllowed(definition, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand) {
		t.Fatal("valid weapon/slot combination rejected")
	}
	if equipmentDefinitionAllowed(definition, equipmentcatalog.KindWeapon, equipmentcatalog.SlotOffHand) {
		t.Fatal("wrong slot unexpectedly allowed")
	}
	if equipmentDefinitionAllowed(definition, equipmentcatalog.KindShield, equipmentcatalog.SlotMainHand) {
		t.Fatal("wrong kind unexpectedly allowed")
	}
}
