package characterstate

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

type InventoryStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

// InventoryState deliberately stays value-comparable because crash replay relies on Snapshot equality.
// JSON strings are canonical Server-internal persistence encodings. Initialized=false remains the
// migration fence for records predating durable inventory.
type InventoryState struct {
	Initialized          bool   `json:"initialized"`
	StacksJSON           string `json:"stacks_json,omitempty"`
	InstancesJSON        string `json:"instances_json,omitempty"`
	MainHand             string `json:"main_hand,omitempty"`
	OffHand              string `json:"off_hand,omitempty"`
	MainHandInstanceJSON string `json:"main_hand_instance_json,omitempty"`
	OffHandInstanceJSON  string `json:"off_hand_instance_json,omitempty"`
}

func NewInventoryStateWithEquipment(stacks []InventoryStack, mainHand, offHand string) (InventoryState, error) {
	return NewInventoryStateWithInstances(stacks, nil, mainHand, offHand, nil, nil)
}

func NewInventoryStateWithInstances(
	stacks []InventoryStack,
	instances []iteminstance.Instance,
	mainHand, offHand string,
	mainHandInstance, offHandInstance *iteminstance.Instance,
) (InventoryState, error) {
	canonicalStacks := append([]InventoryStack(nil), stacks...)
	for index := range canonicalStacks {
		canonicalStacks[index].ItemArchetypeID = strings.TrimSpace(canonicalStacks[index].ItemArchetypeID)
	}
	sort.Slice(canonicalStacks, func(i, j int) bool { return canonicalStacks[i].ItemArchetypeID < canonicalStacks[j].ItemArchetypeID })

	state := InventoryState{Initialized: true, MainHand: strings.TrimSpace(mainHand), OffHand: strings.TrimSpace(offHand)}
	if len(canonicalStacks) > 0 {
		data, err := json.Marshal(canonicalStacks)
		if err != nil { return InventoryState{}, err }
		state.StacksJSON = string(data)
	}

	canonicalInstances, err := canonicalInstanceList(instances)
	if err != nil { return InventoryState{}, err }
	state.InstancesJSON, err = encodeCanonicalInstances(canonicalInstances)
	if err != nil { return InventoryState{}, err }

	if mainHandInstance != nil {
		data, err := iteminstance.CanonicalShapeJSON(*mainHandInstance)
		if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		state.MainHandInstanceJSON = string(data)
	}
	if offHandInstance != nil {
		data, err := iteminstance.CanonicalShapeJSON(*offHandInstance)
		if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		state.OffHandInstanceJSON = string(data)
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

func (state InventoryState) Instances() ([]iteminstance.Instance, error) {
	if state.InstancesJSON == "" { return nil, nil }
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(state.InstancesJSON), &raw); err != nil { return nil, ErrInvalidSnapshot }
	instances := make([]iteminstance.Instance, 0, len(raw))
	for _, encoded := range raw {
		instance, err := iteminstance.DecodeCanonicalShapeJSON(encoded)
		if err != nil { return nil, ErrInvalidSnapshot }
		instances = append(instances, instance)
	}
	return instances, nil
}

func (state InventoryState) MainHandInstance() (iteminstance.Instance, bool, error) {
	if state.MainHandInstanceJSON == "" { return iteminstance.Instance{}, false, nil }
	instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(state.MainHandInstanceJSON))
	if err != nil { return iteminstance.Instance{}, false, ErrInvalidSnapshot }
	return instance, true, nil
}

func (state InventoryState) OffHandInstance() (iteminstance.Instance, bool, error) {
	if state.OffHandInstanceJSON == "" { return iteminstance.Instance{}, false, nil }
	instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(state.OffHandInstanceJSON))
	if err != nil { return iteminstance.Instance{}, false, ErrInvalidSnapshot }
	return instance, true, nil
}

func canonicalInstanceList(instances []iteminstance.Instance) ([]iteminstance.Instance, error) {
	if len(instances) == 0 { return nil, nil }
	canonical := append([]iteminstance.Instance(nil), instances...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })
	for index := range canonical {
		data, err := iteminstance.CanonicalShapeJSON(canonical[index])
		if err != nil { return nil, ErrInvalidSnapshot }
		decoded, err := iteminstance.DecodeCanonicalShapeJSON(data)
		if err != nil { return nil, ErrInvalidSnapshot }
		canonical[index] = decoded
		if index > 0 && canonical[index-1].ID >= canonical[index].ID { return nil, ErrInvalidSnapshot }
	}
	return canonical, nil
}

func encodeCanonicalInstances(instances []iteminstance.Instance) (string, error) {
	if len(instances) == 0 { return "", nil }
	raw := make([]json.RawMessage, 0, len(instances))
	for _, instance := range instances {
		data, err := iteminstance.CanonicalShapeJSON(instance)
		if err != nil { return "", ErrInvalidSnapshot }
		raw = append(raw, json.RawMessage(data))
	}
	data, err := json.Marshal(raw)
	if err != nil { return "", err }
	return string(data), nil
}

func validateInventoryState(state InventoryState) error {
	if !state.Initialized {
		if state.StacksJSON != "" || state.InstancesJSON != "" || strings.TrimSpace(state.MainHand) != "" || strings.TrimSpace(state.OffHand) != "" || state.MainHandInstanceJSON != "" || state.OffHandInstanceJSON != "" {
			return ErrInvalidSnapshot
		}
		return nil
	}
	if state.MainHand != strings.TrimSpace(state.MainHand) || state.OffHand != strings.TrimSpace(state.OffHand) { return ErrInvalidSnapshot }
	if state.MainHand != "" && state.MainHandInstanceJSON != "" { return ErrInvalidSnapshot }
	if state.OffHand != "" && state.OffHandInstanceJSON != "" { return ErrInvalidSnapshot }
	if state.MainHand != "" && state.MainHand == state.OffHand { return ErrInvalidSnapshot }

	stacks, err := state.Stacks()
	if err != nil { return err }
	lastStack := ""
	for _, stack := range stacks {
		id := strings.TrimSpace(stack.ItemArchetypeID)
		if id == "" || id != stack.ItemArchetypeID || stack.Quantity == 0 || (lastStack != "" && id <= lastStack) { return ErrInvalidSnapshot }
		lastStack = id
	}
	if len(stacks) == 0 && state.StacksJSON != "" { return ErrInvalidSnapshot }
	if len(stacks) > 0 {
		data, err := json.Marshal(stacks)
		if err != nil || string(data) != state.StacksJSON { return ErrInvalidSnapshot }
	}

	instances, err := state.Instances()
	if err != nil { return err }
	lastInstanceID := iteminstance.ID("")
	seen := make(map[iteminstance.ID]struct{}, len(instances)+2)
	for _, instance := range instances {
		if lastInstanceID != "" && instance.ID <= lastInstanceID { return ErrInvalidSnapshot }
		lastInstanceID = instance.ID
		if _, duplicate := seen[instance.ID]; duplicate { return ErrInvalidSnapshot }
		seen[instance.ID] = struct{}{}
	}
	if len(instances) == 0 && state.InstancesJSON != "" { return ErrInvalidSnapshot }
	if len(instances) > 0 {
		encoded, err := encodeCanonicalInstances(instances)
		if err != nil || encoded != state.InstancesJSON { return ErrInvalidSnapshot }
	}

	mainInstance, hasMainInstance, err := state.MainHandInstance()
	if err != nil { return err }
	if hasMainInstance {
		if _, duplicate := seen[mainInstance.ID]; duplicate { return ErrInvalidSnapshot }
		seen[mainInstance.ID] = struct{}{}
	}
	offInstance, hasOffInstance, err := state.OffHandInstance()
	if err != nil { return err }
	if hasOffInstance {
		if _, duplicate := seen[offInstance.ID]; duplicate { return ErrInvalidSnapshot }
		seen[offInstance.ID] = struct{}{}
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
	instances, err := state.Instances()
	if err != nil { return InventoryState{}, err }
	mainInstance, hasMainInstance, err := state.MainHandInstance()
	if err != nil { return InventoryState{}, err }
	offInstance, hasOffInstance, err := state.OffHandInstance()
	if err != nil { return InventoryState{}, err }
	var mainPtr, offPtr *iteminstance.Instance
	if hasMainInstance { mainPtr = &mainInstance }
	if hasOffInstance { offPtr = &offInstance }
	return NewInventoryStateWithInstances(stacks, instances, state.MainHand, state.OffHand, mainPtr, offPtr)
}
