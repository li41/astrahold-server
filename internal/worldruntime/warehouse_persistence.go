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

func restoreCharacterWarehouse(state characterstate.WarehouseState) (*warehouse.Storage, error) {
	canonical, err := characterstate.CanonicalWarehouseState(state)
	if err != nil || !canonical.Initialized {
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
