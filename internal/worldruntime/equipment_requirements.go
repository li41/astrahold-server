package worldruntime

import (
	"errors"

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
