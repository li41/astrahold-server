package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

var ErrEquipmentRequirementsNotMet = errors.New("worldruntime: equipment base requirements not met")

func validateEquipmentInstanceBaseRequirements(inv *inventory.Inventory, instanceID iteminstance.ID, base characterstats.Primary) error {
	if inv == nil {
		return inventory.ErrInstanceNotFound
	}
	instance, ok := inv.Instance(instanceID)
	if !ok {
		return inventory.ErrInstanceNotFound
	}
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
	if !ok {
		return ErrEquipmentItemNotAllowed
	}
	if !definition.BaseRequirementsMet(base) {
		return ErrEquipmentRequirementsNotMet
	}
	return nil
}

// validateRestoredEquipmentBaseRequirements re-applies live equip requirements to durable
// equipped instances before a restored character is allowed into the authoritative world. The
// supplied stats are the durable base stats, deliberately excluding affixes, set bonuses, and all
// other equipment-derived effective-stat contributions.
func validateRestoredEquipmentBaseRequirements(state characterstate.InventoryState, base characterstats.Primary) error {
	if !state.Initialized {
		return nil
	}
	equipped, err := state.EquipmentInstances()
	if err != nil {
		return err
	}
	for _, slot := range equipped {
		instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(slot.ItemInstanceJSON))
		if err != nil {
			return err
		}
		definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
		if !ok {
			return ErrEquipmentItemNotAllowed
		}
		if !definition.BaseRequirementsMet(base) {
			return ErrEquipmentRequirementsNotMet
		}
	}
	return nil
}
