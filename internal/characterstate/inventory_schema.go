package characterstate

func (state InventoryState) HasItemInstances() bool {
	return state.InstancesJSON != "" || state.EquipmentInstancesJSON != "" || state.MainHandInstanceJSON != "" || state.OffHandInstanceJSON != ""
}

func (state InventoryState) HasGenericEquipment() bool {
	return state.EquipmentJSON != "" || state.EquipmentInstancesJSON != ""
}
