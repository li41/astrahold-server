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

// InventoryState is the durable inventory/equipment aggregate. Snapshot.Inventory == nil
// means a legacy v1-v3 state (or old save-journal command) that never captured inventory.
// A non-nil empty InventoryState is therefore a genuinely empty v4 inventory.
type InventoryState struct {
	Stacks   []InventoryStack `json:"stacks,omitempty"`
	MainHand string           `json:"main_hand,omitempty"`
}

func validateInventoryState(state *InventoryState) error {
	if state == nil {
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
// Store and save-journal bytes. nil is preserved because it is the legacy migration fence.
func CanonicalInventoryState(state *InventoryState) *InventoryState {
	if state == nil {
		return nil
	}
	out := &InventoryState{MainHand: strings.TrimSpace(state.MainHand)}
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

// SnapshotsEqual provides semantic equality for crash-replay idempotence. Snapshot stays
// comparable for legacy callers, but v4 inventory pointers require content comparison.
func SnapshotsEqual(a, b Snapshot) bool {
	aInventory, bInventory := a.Inventory, b.Inventory
	a.Inventory, b.Inventory = nil, nil
	if a != b {
		return false
	}
	if aInventory == nil || bInventory == nil {
		return aInventory == nil && bInventory == nil
	}
	if aInventory.MainHand != bInventory.MainHand || len(aInventory.Stacks) != len(bInventory.Stacks) {
		return false
	}
	for index := range aInventory.Stacks {
		if aInventory.Stacks[index] != bInventory.Stacks[index] {
			return false
		}
	}
	return true
}
