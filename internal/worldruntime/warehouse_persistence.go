package worldruntime

import (
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/warehouse"
)

func durableWarehouseState(storage *warehouse.Storage) characterstate.WarehouseState {
	if storage == nil {
		return characterstate.EmptyWarehouseState()
	}
	stacks := storage.Snapshot()
	items := make([]characterstate.WarehouseStack, 0, len(stacks))
	for _, stack := range stacks {
		items = append(items, characterstate.WarehouseStack{
			ItemArchetypeID: stack.ItemArchetypeID,
			Quantity:        stack.Quantity,
		})
	}
	return characterstate.WarehouseState{Initialized: true, Items: items}
}

func canonicalWarehouseState(state characterstate.WarehouseState) (characterstate.WarehouseState, bool) {
	// CharacterRestore is a trusted Server-internal compatibility boundary. Records written before
	// warehouse schema v15 legitimately carry the zero value, and older internal bootstrap fixtures
	// do the same. Normalize only that exact zero value to an initialized empty personal warehouse.
	// Durable current-schema records remain strict in characterstate.validateSnapshotForSchema.
	if !state.Initialized && len(state.Items) == 0 {
		return characterstate.EmptyWarehouseState(), true
	}
	canonical, err := characterstate.CanonicalWarehouseState(state)
	if err != nil || !canonical.Initialized || len(canonical.Items) != len(state.Items) {
		return characterstate.WarehouseState{}, false
	}
	for index := range canonical.Items {
		if canonical.Items[index] != state.Items[index] {
			return characterstate.WarehouseState{}, false
		}
	}
	return canonical, true
}

func restoreCharacterWarehouse(state characterstate.WarehouseState) (*warehouse.Storage, error) {
	canonical, ok := canonicalWarehouseState(state)
	if !ok {
		return nil, ErrCharacterRestoreInvalid
	}
	stacks := make([]warehouse.Stack, 0, len(canonical.Items))
	for _, item := range canonical.Items {
		stacks = append(stacks, warehouse.Stack{
			ItemArchetypeID: item.ItemArchetypeID,
			Quantity:        item.Quantity,
		})
	}
	storage, err := warehouse.Restore(stacks)
	if err != nil {
		return nil, ErrCharacterRestoreInvalid
	}
	return storage, nil
}
