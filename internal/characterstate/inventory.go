package characterstate

import (
	"sort"
	"strings"
)

// InventoryStack is stable durable gameplay truth for one unequipped item stack.
// Presentation metadata and derived carry-weight values are intentionally excluded.
type InventoryStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

// InventoryState is the durable inventory/equipment aggregate. Initialized distinguishes
// a genuinely empty v4 inventory from legacy v1-v3 records and old journal entries that
// predate inventory persistence.
type InventoryState struct {
	Initialized bool             `json:"initialized"`
	Stacks      []InventoryStack `json:"stacks,omitempty"`
	MainHand    string           `json:"main_hand,omitempty"`
}

func validateInventoryState(state InventoryState) error {
	if !state.Initialized {
		if len(state.Stacks) != 0 || strings.TrimSpace(state.MainHand) != "" {
			return ErrInvalidSnapshot
		}
		return nil
	}
	seen := make(map[string]struct{}, len(state.Stacks))
	last := ""
	for _, stack := range state.Stacks {
		id := strings.TrimSpace(stack.ItemArchetypeID)
		if id == "" || id != stack.ItemArchetypeID || stack.Quantity == 0 {
			return ErrInvalidSnapshot
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidSnapshot
		}
		seen[id] = struct{}{}
		if last != "" && id <= last {
			return ErrInvalidSnapshot
		}
		last = id
	}
	if state.MainHand != strings.TrimSpace(state.MainHand) {
		return ErrInvalidSnapshot
	}
	return nil
}

// CanonicalInventoryState defensively copies and sorts stack identity for deterministic
// Store and save-journal bytes. Callers still own gameplay legality validation.
func CanonicalInventoryState(state InventoryState) InventoryState {
	out := InventoryState{Initialized: state.Initialized, MainHand: strings.TrimSpace(state.MainHand)}
	if len(state.Stacks) == 0 {
		return out
	}
	out.Stacks = append([]InventoryStack(nil), state.Stacks...)
	for index := range out.Stacks {
		out.Stacks[index].ItemArchetypeID = strings.TrimSpace(out.Stacks[index].ItemArchetypeID)
	}
	sort.Slice(out.Stacks, func(i, j int) bool { return out.Stacks[i].ItemArchetypeID < out.Stacks[j].ItemArchetypeID })
	return out
}
