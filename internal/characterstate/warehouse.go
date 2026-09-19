package characterstate

import (
	"sort"
	"strings"
)

type WarehouseStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

// WarehouseState is durable stack-item storage owned by the character. Initialized distinguishes
// the formal warehouse schema from legacy records that predate warehouse persistence.
type WarehouseState struct {
	Initialized bool             `json:"initialized"`
	Items       []WarehouseStack `json:"items,omitempty"`
}

func EmptyWarehouseState() WarehouseState {
	return WarehouseState{Initialized: true, Items: []WarehouseStack{}}
}

func CanonicalWarehouseState(state WarehouseState) (WarehouseState, error) {
	if !state.Initialized {
		if len(state.Items) != 0 {
			return WarehouseState{}, ErrInvalidSnapshot
		}
		return WarehouseState{}, nil
	}
	items := append([]WarehouseStack(nil), state.Items...)
	for _, item := range items {
		if item.ItemArchetypeID == "" || item.ItemArchetypeID != strings.TrimSpace(item.ItemArchetypeID) || item.Quantity == 0 {
			return WarehouseState{}, ErrInvalidSnapshot
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ItemArchetypeID < items[j].ItemArchetypeID })
	for i := 1; i < len(items); i++ {
		if items[i-1].ItemArchetypeID == items[i].ItemArchetypeID {
			return WarehouseState{}, ErrInvalidSnapshot
		}
	}
	if len(items) == 0 {
		items = []WarehouseStack{}
	}
	return WarehouseState{Initialized: true, Items: items}, nil
}
