package characterstate

// HasItemInstances reports whether durable inventory state contains unique equipment instance
// data. The schema gate uses this instead of inferring support from archetype stack fields.
func (state InventoryState) HasItemInstances() bool {
	return state.InstancesJSON != "" || state.MainHandInstanceJSON != "" || state.OffHandInstanceJSON != ""
}
