package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

var ErrEquipmentHandConflict = errors.New("worldruntime: equipment hand requirement conflict")

func equipmentHandRequirement(itemArchetypeID string) (equipmentcatalog.HandRequirement, bool) {
	if itemArchetypeID == trainingBladeArchetypeID { return equipmentcatalog.HandRequirementOneHand, true }
	return defaultEquipmentCatalog.HandRequirementForItem(itemArchetypeID)
}
func equippedMainHandArchetypeID(inv *inventory.Inventory) string { if inv == nil { return "" }; return inv.MainHand() }
func equippedOffHandArchetypeID(inv *inventory.Inventory) string { if inv == nil { return "" }; return inv.OffHand() }
func validateEquipmentHandCombination(mainHandItemArchetypeID, offHandItemArchetypeID string) error {
	if mainHandItemArchetypeID == "" || offHandItemArchetypeID == "" { return nil }
	requirement, ok := equipmentHandRequirement(mainHandItemArchetypeID); if !ok { return ErrEquipmentItemNotAllowed }
	if requirement == equipmentcatalog.HandRequirementTwoHand { return ErrEquipmentHandConflict }
	return nil
}
func validateInventoryHandCombination(inv *inventory.Inventory) error { return validateEquipmentHandCombination(equippedMainHandArchetypeID(inv), equippedOffHandArchetypeID(inv)) }
func validateInventoryStateHandCombination(state characterstate.InventoryState) error {
	equipment, err := state.Equipment(); if err != nil { return err }
	mainHand, offHand := "", ""
	for _, item := range equipment { if item.Slot == "main_hand" { mainHand = item.ItemArchetypeID }; if item.Slot == "off_hand" { offHand = item.ItemArchetypeID } }
	instances, err := state.EquipmentInstances(); if err != nil { return err }
	for _, item := range instances {
		instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(item.ItemInstanceJSON)); if err != nil { return err }
		if item.Slot == "main_hand" { mainHand = instance.ItemArchetypeID }; if item.Slot == "off_hand" { offHand = instance.ItemArchetypeID }
	}
	return validateEquipmentHandCombination(mainHand, offHand)
}
func validateMainHandEquipmentCompatibility(inv *inventory.Inventory, itemArchetypeID string) error { return validateEquipmentHandCombination(itemArchetypeID, equippedOffHandArchetypeID(inv)) }
func validateOffHandEquipmentCompatibility(inv *inventory.Inventory) error { return validateEquipmentHandCombination(equippedMainHandArchetypeID(inv), "occupied") }

func applyEquipArchetype(inv *inventory.Inventory, slot inventory.EquipmentSlot, itemArchetypeID string) error {
	if !lowTierEquipmentAllowed(slot, itemArchetypeID) { return ErrEquipmentItemNotAllowed }
	if slot == inventory.SlotMainHand { if err := validateMainHandEquipmentCompatibility(inv, itemArchetypeID); err != nil { return err } }
	if slot == inventory.SlotOffHand { if err := validateOffHandEquipmentCompatibility(inv); err != nil { return err } }
	return inv.Equip(slot, itemArchetypeID)
}
