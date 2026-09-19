package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidItemDropPayload = errors.New("worldruntime: invalid item drop payload")

type itemDropPayload struct {
	Kind            loot.DropKind
	ItemArchetypeID string
	Quantity        uint32
	Instance        iteminstance.Instance
}

func stackItemDropPayload(itemArchetypeID string, quantity uint32) itemDropPayload {
	return itemDropPayload{
		Kind:            loot.DropKindStack,
		ItemArchetypeID: itemArchetypeID,
		Quantity:        quantity,
	}
}

func instanceItemDropPayload(instance iteminstance.Instance) itemDropPayload {
	return itemDropPayload{
		Kind:            loot.DropKindEquipmentInstance,
		ItemArchetypeID: instance.ItemArchetypeID,
		Quantity:        1,
		Instance:        instance,
	}
}

func (r *Runtime) validateItemDropPayload(payload itemDropPayload) error {
	if payload.ItemArchetypeID == "" || payload.Quantity == 0 {
		return ErrInvalidItemDropPayload
	}
	switch payload.Kind {
	case loot.DropKindStack:
		if payload.Instance.ID != "" || payload.Instance.ItemArchetypeID != "" {
			return ErrInvalidItemDropPayload
		}
	case loot.DropKindEquipmentInstance:
		if payload.Quantity != 1 || payload.Instance.ID == "" || payload.Instance.ItemArchetypeID != payload.ItemArchetypeID {
			return ErrInvalidItemDropPayload
		}
		if err := validateDurableEquipmentInstance(payload.Instance, "", ""); err != nil {
			return ErrInvalidItemDropPayload
		}
	default:
		return ErrInvalidItemDropPayload
	}
	return nil
}

func (r *Runtime) itemDropPayloadForEntity(dropID world.EntityID, entity world.EntityState) (itemDropPayload, error) {
	if r != nil {
		if payload, ok := r.itemDropPayloads[dropID]; ok {
			if payload.ItemArchetypeID != entity.ArchetypeID {
				return itemDropPayload{}, ErrInvalidItemDropPayload
			}
			if err := r.validateItemDropPayload(payload); err != nil {
				return itemDropPayload{}, err
			}
			return payload, nil
		}
	}
	// Historical/raw item-drop producers carry no private payload and retain the original one-stack
	// semantics. This keeps focused tests and non-loot callers backwards compatible.
	payload := stackItemDropPayload(entity.ArchetypeID, 1)
	if err := r.validateItemDropPayload(payload); err != nil {
		return itemDropPayload{}, err
	}
	return payload, nil
}

func (r *Runtime) grantItemDropPayload(inv *inventory.Inventory, payload itemDropPayload) error {
	if inv == nil {
		return inventory.ErrFull
	}
	if err := r.validateItemDropPayload(payload); err != nil {
		return err
	}
	switch payload.Kind {
	case loot.DropKindStack:
		return inv.Add(payload.ItemArchetypeID, payload.Quantity)
	case loot.DropKindEquipmentInstance:
		return inv.AddInstance(payload.Instance)
	default:
		return ErrInvalidItemDropPayload
	}
}

func (r *Runtime) removeItemDrop(dropID world.EntityID) {
	if r == nil || dropID == 0 {
		return
	}
	r.world.Remove(dropID)
	delete(r.itemDropPayloads, dropID)
	delete(r.itemDropExpireTick, dropID)
}
