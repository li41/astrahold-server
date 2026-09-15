package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
)

var ErrEquipmentHandConflict = errors.New("worldruntime: equipment hand requirement conflict")

func equipmentHandRequirement(itemArchetypeID string) (equipmentcatalog.HandRequirement, bool) {
	if itemArchetypeID == trainingBladeArchetypeID {
		return equipmentcatalog.HandRequirementOneHand, true
	}
	return defaultEquipmentCatalog.HandRequirementForItem(itemArchetypeID)
}

func equippedMainHandArchetypeID(inv *inventory.Inventory) string {
	if inv == nil {
		return ""
	}
	if instance, ok := inv.MainHandInstance(); ok {
		return instance.ItemArchetypeID
	}
	return inv.MainHand()
}

func equippedOffHandArchetypeID(inv *inventory.Inventory) string {
	if inv == nil {
		return ""
	}
	if instance, ok := inv.OffHandInstance(); ok {
		return instance.ItemArchetypeID
	}
	return inv.OffHand()
}

func validateEquipmentHandCombination(mainHandItemArchetypeID, offHandItemArchetypeID string) error {
	if mainHandItemArchetypeID == "" || offHandItemArchetypeID == "" {
		return nil
	}
	requirement, ok := equipmentHandRequirement(mainHandItemArchetypeID)
	if !ok {
		return ErrEquipmentItemNotAllowed
	}
	if requirement == equipmentcatalog.HandRequirementTwoHand {
		return ErrEquipmentHandConflict
	}
	return nil
}

func validateInventoryHandCombination(inv *inventory.Inventory) error {
	return validateEquipmentHandCombination(equippedMainHandArchetypeID(inv), equippedOffHandArchetypeID(inv))
}

func validateInventoryStateHandCombination(state characterstate.InventoryState) error {
	mainHand := state.MainHand
	if instance, ok, err := state.MainHandInstance(); err != nil {
		return err
	} else if ok {
		mainHand = instance.ItemArchetypeID
	}
	offHand := state.OffHand
	if instance, ok, err := state.OffHandInstance(); err != nil {
		return err
	} else if ok {
		offHand = instance.ItemArchetypeID
	}
	return validateEquipmentHandCombination(mainHand, offHand)
}

func validateMainHandEquipmentCompatibility(inv *inventory.Inventory, itemArchetypeID string) error {
	return validateEquipmentHandCombination(itemArchetypeID, equippedOffHandArchetypeID(inv))
}

func validateOffHandEquipmentCompatibility(inv *inventory.Inventory) error {
	return validateEquipmentHandCombination(equippedMainHandArchetypeID(inv), "occupied")
}

func applyEquipArchetype(inv *inventory.Inventory, slot protocol.EquipmentSlot, itemArchetypeID string) error {
	switch slot {
	case protocol.EquipmentSlotMainHand:
		if !mainHandItemAllowed(itemArchetypeID) {
			return ErrEquipmentItemNotAllowed
		}
		if err := validateMainHandEquipmentCompatibility(inv, itemArchetypeID); err != nil {
			return err
		}
		return inv.EquipMainHand(itemArchetypeID)
	case protocol.EquipmentSlotOffHand:
		if !offHandItemAllowed(itemArchetypeID) {
			return ErrEquipmentItemNotAllowed
		}
		if err := validateOffHandEquipmentCompatibility(inv); err != nil {
			return err
		}
		return inv.EquipOffHand(itemArchetypeID)
	default:
		return errors.New("worldruntime: invalid equipment slot")
	}
}
