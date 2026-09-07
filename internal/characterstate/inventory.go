package characterstate

import (
	"encoding/json"
	"sort"
	"strings"
)

// InventoryStack is stable durable gameplay truth for one unequipped item stack.
type InventoryStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

// InventoryState deliberately stays value-comparable because character-state crash replay
// already relies on Snapshot equality. StacksJSON is canonical Server-internal persistence
// encoding; callers use NewInventoryState/Stacks rather than treating it as gameplay data.
// Initialized=false is the migration fence for v1-v3 records and old save-journal entries.
type InventoryState struct {
	Initialized bool   `json:"initialized"`
	StacksJSON  string `json:"stacks_json,omitempty"`
	MainHand    string `json:"main_hand,omitempty"`
}

func NewInventoryState(stacks []InventoryStack, mainHand string) (InventoryState, error) {
	canonical := append([]InventoryStack(nil), stacks...)
	for index := range canonical {
		canonical[index].ItemArchetypeID = strings.TrimSpace(canonical[index].ItemArchetypeID)
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ItemArchetypeID < canonical[j].ItemArchetypeID })
	state := InventoryState{Initialized: true, MainHand: strings.TrimSpace(mainHand)}
	if len(canonical) > 0 {
		data, err := json.Marshal(canonical)
		if err != nil {
			return InventoryState{}, err
		}
		state.StacksJSON = string(data)
	}
	if err := validateInventoryState(state); err != nil {
		return InventoryState{}, err
	}
	return state, nil
}

func (state InventoryState) Stacks() ([]InventoryStack, error) {
	if state.StacksJSON == "" {
		return nil, nil
	}
	var stacks []InventoryStack
	if err := json.Unmarshal([]byte(state.StacksJSON), &stacks); err != nil {
		return nil, ErrInvalidSnapshot
	}
	return stacks, nil
}

func validateInventoryState(state InventoryState) error {
	if !state.Initialized {
		if state.StacksJSON != "" || strings.TrimSpace(state.MainHand) != "" {
			return ErrInvalidSnapshot
		}
		return nil
	}
	if state.MainHand != strings.TrimSpace(state.MainHand) {
		return ErrInvalidSnapshot
	}
	stacks, err := state.Stacks()
	if err != nil {
		return err
	}
	last := ""
	for _, stack := range stacks {
		id := strings.TrimSpace(stack.ItemArchetypeID)
		if id == "" || id != stack.ItemArchetypeID || stack.Quantity == 0 || (last != "" && id <= last) {
			return ErrInvalidSnapshot
		}
		last = id
	}
	if len(stacks) == 0 {
		if state.StacksJSON != "" {
			return ErrInvalidSnapshot
		}
		return nil
	}
	data, err := json.Marshal(stacks)
	if err != nil || string(data) != state.StacksJSON {
		return ErrInvalidSnapshot
	}
	return nil
}

func CanonicalInventoryState(state InventoryState) (InventoryState, error) {
	if !state.Initialized {
		if err := validateInventoryState(state); err != nil { return InventoryState{}, err }
		return InventoryState{}, nil
	}
	stacks, err := state.Stacks()
	if err != nil { return InventoryState{}, err }
	return NewInventoryState(stacks, state.MainHand)
}
