package inventory

import (
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

type EquippedInstance struct {
	Slot EquipmentSlot
	Item iteminstance.Instance
}

// AddInstance adds one unique equipment instance to the authoritative unequipped inventory.
// The caller must have already validated the instance against its equipment definition.
func (i *Inventory) AddInstance(instance iteminstance.Instance) error {
	if i == nil {
		return ErrFull
	}
	instanceID := iteminstance.ID(strings.TrimSpace(string(instance.ID)))
	archetypeID := strings.TrimSpace(instance.ItemArchetypeID)
	if instanceID == "" || archetypeID == "" || instanceID != instance.ID || archetypeID != instance.ItemArchetypeID {
		return ErrInvalidInstance
	}
	if _, exists := i.instances[instanceID]; exists || i.instanceEquipped(instanceID) {
		return ErrInstanceExists
	}
	if i.unequippedEntryCount() >= i.maxStacks {
		return ErrFull
	}
	addedWeight := i.weightFor(archetypeID, 1)
	if i.maxWeight > 0 && (addedWeight > i.maxWeight || i.currentWeight > i.maxWeight-addedWeight) {
		return ErrWeightExceeded
	}
	i.instances[instanceID] = cloneInstance(instance)
	i.currentWeight += addedWeight
	i.revision++
	return nil
}

func (i *Inventory) instanceEquipped(instanceID iteminstance.ID) bool {
	if i == nil {
		return false
	}
	for _, slot := range equipmentSlots {
		if i.equippedInstances[slot].ID == instanceID {
			return true
		}
	}
	return false
}

func (i *Inventory) RemoveInstance(instanceID iteminstance.ID) (iteminstance.Instance, error) {
	if i == nil {
		return iteminstance.Instance{}, ErrInstanceNotFound
	}
	instanceID = iteminstance.ID(strings.TrimSpace(string(instanceID)))
	if instanceID == "" {
		return iteminstance.Instance{}, ErrInvalidInstance
	}
	instance, exists := i.instances[instanceID]
	if !exists {
		return iteminstance.Instance{}, ErrInstanceNotFound
	}
	delete(i.instances, instanceID)
	i.removeInstanceWeight(instance)
	i.revision++
	return cloneInstance(instance), nil
}

// RemoveOwnedInstance destroys one exact unique item whether it is unequipped or equipped.
// Equipped destruction also advances equipmentRevision so derived stats and replication immediately
// observe the empty slot. It never falls back to archetype matching.
func (i *Inventory) RemoveOwnedInstance(instanceID iteminstance.ID) (iteminstance.Instance, bool, error) {
	if i == nil {
		return iteminstance.Instance{}, false, ErrInstanceNotFound
	}
	instanceID = iteminstance.ID(strings.TrimSpace(string(instanceID)))
	if instanceID == "" {
		return iteminstance.Instance{}, false, ErrInvalidInstance
	}
	if instance, exists := i.instances[instanceID]; exists {
		delete(i.instances, instanceID)
		i.removeInstanceWeight(instance)
		i.revision++
		return cloneInstance(instance), false, nil
	}
	for _, slot := range equipmentSlots {
		instance := i.equippedInstances[slot]
		if instance.ID != instanceID {
			continue
		}
		delete(i.equippedInstances, slot)
		i.removeInstanceWeight(instance)
		i.revision++
		i.equipmentRevision++
		return cloneInstance(instance), true, nil
	}
	return iteminstance.Instance{}, false, ErrInstanceNotFound
}

func (i *Inventory) removeInstanceWeight(instance iteminstance.Instance) {
	removedWeight := i.weightFor(instance.ItemArchetypeID, 1)
	if removedWeight >= i.currentWeight {
		i.currentWeight = 0
		return
	}
	i.currentWeight -= removedWeight
}

func (i *Inventory) Instance(instanceID iteminstance.ID) (iteminstance.Instance, bool) {
	if i == nil {
		return iteminstance.Instance{}, false
	}
	instance, ok := i.instances[instanceID]
	if !ok {
		return iteminstance.Instance{}, false
	}
	return cloneInstance(instance), true
}

// OwnedInstance resolves one exact unique instance whether it is currently carried or equipped.
func (i *Inventory) OwnedInstance(instanceID iteminstance.ID) (iteminstance.Instance, bool, bool) {
	if i == nil {
		return iteminstance.Instance{}, false, false
	}
	instanceID = iteminstance.ID(strings.TrimSpace(string(instanceID)))
	if instance, ok := i.instances[instanceID]; ok {
		return cloneInstance(instance), false, true
	}
	for _, slot := range equipmentSlots {
		if instance := i.equippedInstances[slot]; instance.ID == instanceID {
			return cloneInstance(instance), true, true
		}
	}
	return iteminstance.Instance{}, false, false
}

// ReplaceOwnedInstance atomically replaces mutable state on one already-owned unique instance.
// Identity and archetype are immutable; equipped replacements also advance equipment revision so
// derived gameplay stats and replication observe the enhancement immediately.
func (i *Inventory) ReplaceOwnedInstance(instance iteminstance.Instance) (bool, error) {
	if i == nil {
		return false, ErrInstanceNotFound
	}
	if err := iteminstance.ValidateShape(instance); err != nil {
		return false, ErrInvalidInstance
	}
	if current, ok := i.instances[instance.ID]; ok {
		if current.ItemArchetypeID != instance.ItemArchetypeID {
			return false, ErrInvalidInstance
		}
		i.instances[instance.ID] = cloneInstance(instance)
		i.revision++
		return false, nil
	}
	for _, slot := range equipmentSlots {
		current := i.equippedInstances[slot]
		if current.ID != instance.ID {
			continue
		}
		if current.ItemArchetypeID != instance.ItemArchetypeID {
			return false, ErrInvalidInstance
		}
		i.equippedInstances[slot] = cloneInstance(instance)
		i.revision++
		i.equipmentRevision++
		return true, nil
	}
	return false, ErrInstanceNotFound
}

func (i *Inventory) InstanceSnapshot() []iteminstance.Instance {
	if i == nil || len(i.instances) == 0 {
		return nil
	}
	out := make([]iteminstance.Instance, 0, len(i.instances))
	for _, instance := range i.instances {
		out = append(out, cloneInstance(instance))
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

func (i *Inventory) EquippedInstance(slot EquipmentSlot) (iteminstance.Instance, bool) {
	if i == nil || !ValidEquipmentSlot(slot) {
		return iteminstance.Instance{}, false
	}
	instance := i.equippedInstances[slot]
	if instance.ID == "" {
		return iteminstance.Instance{}, false
	}
	return cloneInstance(instance), true
}

func (i *Inventory) EquippedInstanceSnapshot() []EquippedInstance {
	if i == nil {
		return nil
	}
	out := make([]EquippedInstance, 0, len(equipmentSlots))
	for _, slot := range equipmentSlots {
		if instance := i.equippedInstances[slot]; instance.ID != "" {
			out = append(out, EquippedInstance{Slot: slot, Item: cloneInstance(instance)})
		}
	}
	return out
}

func (i *Inventory) MainHandInstance() (iteminstance.Instance, bool) {
	return i.EquippedInstance(SlotMainHand)
}
func (i *Inventory) OffHandInstance() (iteminstance.Instance, bool) {
	return i.EquippedInstance(SlotOffHand)
}

func (i *Inventory) EquipInstance(slot EquipmentSlot, instanceID iteminstance.ID) error {
	if i == nil {
		return ErrInstanceNotFound
	}
	if !ValidEquipmentSlot(slot) {
		return ErrInvalidEquipmentSlot
	}
	if i.equipped[slot] != "" || i.equippedInstances[slot].ID != "" {
		return ErrEquipmentSlotOccupied
	}
	instance, ok := i.instances[instanceID]
	if !ok {
		return ErrInstanceNotFound
	}
	delete(i.instances, instanceID)
	i.equippedInstances[slot] = cloneInstance(instance)
	i.revision++
	i.equipmentRevision++
	return nil
}

func (i *Inventory) UnequipInstance(slot EquipmentSlot) (iteminstance.Instance, error) {
	if i == nil {
		return iteminstance.Instance{}, ErrEquipmentSlotEmpty
	}
	if !ValidEquipmentSlot(slot) {
		return iteminstance.Instance{}, ErrInvalidEquipmentSlot
	}
	instance := i.equippedInstances[slot]
	if instance.ID == "" {
		return iteminstance.Instance{}, ErrEquipmentSlotEmpty
	}
	if i.unequippedEntryCount() >= i.maxStacks {
		return iteminstance.Instance{}, ErrFull
	}
	copy := cloneInstance(instance)
	i.instances[copy.ID] = cloneInstance(copy)
	delete(i.equippedInstances, slot)
	i.revision++
	i.equipmentRevision++
	return copy, nil
}

func (i *Inventory) EquipMainHandInstance(instanceID iteminstance.ID) error {
	return i.EquipInstance(SlotMainHand, instanceID)
}
func (i *Inventory) UnequipMainHandInstance() (iteminstance.Instance, error) {
	return i.UnequipInstance(SlotMainHand)
}
func (i *Inventory) EquipOffHandInstance(instanceID iteminstance.ID) error {
	return i.EquipInstance(SlotOffHand, instanceID)
}
func (i *Inventory) UnequipOffHandInstance() (iteminstance.Instance, error) {
	return i.UnequipInstance(SlotOffHand)
}

func cloneInstance(instance iteminstance.Instance) iteminstance.Instance {
	copy := instance
	if len(instance.Affixes) > 0 {
		copy.Affixes = append(copy.Affixes[:0:0], instance.Affixes...)
	}
	return copy
}
