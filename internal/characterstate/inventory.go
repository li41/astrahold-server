package characterstate

import (
	"encoding/json"
	"sort"
	"strings"
)

type InventoryStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

// InventoryState deliberately stays value-comparable because crash replay relies on Snapshot equality.
// StacksJSON is canonical Server-internal persistence encoding. Initialized=false remains the migration
// fence for records predating durable inventory.
type InventoryState struct {
	Initialized bool   `json:"initialized"`
	StacksJSON  string `json:"stacks_json,omitempty"`
	MainHand    string `json:"main_hand,omitempty"`
	OffHand     string `json:"off_hand,omitempty"`
}

// NewInventoryState preserves the pre-v5 call surface for main-hand-only callers.
func NewInventoryState(stacks []InventoryStack, mainHand string) (InventoryState, error) {
	return NewInventoryStateWithEquipment(stacks, mainHand, "")
}

func NewInventoryStateWithEquipment(stacks []InventoryStack, mainHand, offHand string) (InventoryState, error) {
	canonical := append([]InventoryStack(nil), stacks...)
	for index := range canonical { canonical[index].ItemArchetypeID = strings.TrimSpace(canonical[index].ItemArchetypeID) }
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ItemArchetypeID < canonical[j].ItemArchetypeID })
	state := InventoryState{Initialized: true, MainHand: strings.TrimSpace(mainHand), OffHand: strings.TrimSpace(offHand)}
	if len(canonical) > 0 {
		data, err := json.Marshal(canonical)
		if err != nil { return InventoryState{}, err }
		state.StacksJSON = string(data)
	}
	if err := validateInventoryState(state); err != nil { return InventoryState{}, err }
	return state, nil
}

func (state InventoryState) Stacks() ([]InventoryStack, error) {
	if state.StacksJSON == "" { return nil, nil }
	var stacks []InventoryStack
	if err := json.Unmarshal([]byte(state.StacksJSON), &stacks); err != nil { return nil, ErrInvalidSnapshot }
	return stacks, nil
}

func validateInventoryState(state InventoryState) error {
	if !state.Initialized {
		if state.StacksJSON != "" || strings.TrimSpace(state.MainHand) != "" || strings.TrimSpace(state.OffHand) != "" { return ErrInvalidSnapshot }
		return nil
	}
	if state.MainHand != strings.TrimSpace(state.MainHand) || state.OffHand != strings.TrimSpace(state.OffHand) { return ErrInvalidSnapshot }
	if state.MainHand != "" && state.MainHand == state.OffHand { return ErrInvalidSnapshot }
	stacks, err := state.Stacks()
	if err != nil { return err }
	last := ""
	for _, stack := range stacks {
		id := strings.TrimSpace(stack.ItemArchetypeID)
		if id == "" || id != stack.ItemArchetypeID || stack.Quantity == 0 || (last != "" && id <= last) { return ErrInvalidSnapshot }
		last = id
	}
	if len(stacks) == 0 {
		if state.StacksJSON != "" { return ErrInvalidSnapshot }
		return nil
	}
	data, err := json.Marshal(stacks)
	if err != nil || string(data) != state.StacksJSON { return ErrInvalidSnapshot }
	return nil
}

func CanonicalInventoryState(state InventoryState) (InventoryState, error) {
	if !state.Initialized {
		if err := validateInventoryState(state); err != nil { return InventoryState{}, err }
		return InventoryState{}, nil
	}
	stacks, err := state.Stacks()
	if err != nil { return InventoryState{}, err }
	return NewInventoryStateWithEquipment(stacks, state.MainHand, state.OffHand)
}
