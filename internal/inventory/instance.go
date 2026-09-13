package inventory

import (
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

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
	if _, exists := i.instances[instanceID]; exists || i.mainHandInstance.ID == instanceID || i.offHandInstance.ID == instanceID {
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

// RemoveInstance removes an unequipped unique equipment instance.
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
	removedWeight := i.weightFor(instance.ItemArchetypeID, 1)
	if removedWeight >= i.currentWeight {
		i.currentWeight = 0
	} else {
		i.currentWeight -= removedWeight
	}
	i.revision++
	return cloneInstance(instance), nil
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

func (i *Inventory) MainHandInstance() (iteminstance.Instance, bool) {
	if i == nil || i.mainHandInstance.ID == "" {
		return iteminstance.Instance{}, false
	}
	return cloneInstance(i.mainHandInstance), true
}

func (i *Inventory) OffHandInstance() (iteminstance.Instance, bool) {
	if i == nil || i.offHandInstance.ID == "" {
		return iteminstance.Instance{}, false
	}
	return cloneInstance(i.offHandInstance), true
}

func (i *Inventory) EquipMainHandInstance(instanceID iteminstance.ID) error {
	if i == nil {
		return ErrInstanceNotFound
	}
	if i.mainHand != "" || i.mainHandInstance.ID != "" {
		return ErrEquipmentSlotOccupied
	}
	instance, ok := i.instances[instanceID]
	if !ok {
		return ErrInstanceNotFound
	}
	delete(i.instances, instanceID)
	i.mainHandInstance = cloneInstance(instance)
	i.revision++
	i.equipmentRevision++
	return nil
}

func (i *Inventory) UnequipMainHandInstance() (iteminstance.Instance, error) {
	if i == nil || i.mainHandInstance.ID == "" {
		return iteminstance.Instance{}, ErrEquipmentSlotEmpty
	}
	if i.unequippedEntryCount() >= i.maxStacks {
		return iteminstance.Instance{}, ErrFull
	}
	instance := cloneInstance(i.mainHandInstance)
	i.instances[instance.ID] = cloneInstance(instance)
	i.mainHandInstance = iteminstance.Instance{}
	i.revision++
	i.equipmentRevision++
	return instance, nil
}

func (i *Inventory) EquipOffHandInstance(instanceID iteminstance.ID) error {
	if i == nil {
		return ErrInstanceNotFound
	}
	if i.offHand != "" || i.offHandInstance.ID != "" {
		return ErrEquipmentSlotOccupied
	}
	instance, ok := i.instances[instanceID]
	if !ok {
		return ErrInstanceNotFound
	}
	delete(i.instances, instanceID)
	i.offHandInstance = cloneInstance(instance)
	i.revision++
	i.equipmentRevision++
	return nil
}

func (i *Inventory) UnequipOffHandInstance() (iteminstance.Instance, error) {
	if i == nil || i.offHandInstance.ID == "" {
		return iteminstance.Instance{}, ErrEquipmentSlotEmpty
	}
	if i.unequippedEntryCount() >= i.maxStacks {
		return iteminstance.Instance{}, ErrFull
	}
	instance := cloneInstance(i.offHandInstance)
	i.instances[instance.ID] = cloneInstance(instance)
	i.offHandInstance = iteminstance.Instance{}
	i.revision++
	i.equipmentRevision++
	return instance, nil
}

func cloneInstance(instance iteminstance.Instance) iteminstance.Instance {
	copy := instance
	if len(instance.Affixes) > 0 {
		copy.Affixes = append(copy.Affixes[:0:0], instance.Affixes...)
	}
	return copy
}
