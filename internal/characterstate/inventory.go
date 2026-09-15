package characterstate

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

type InventoryStack struct { ItemArchetypeID string `json:"item_archetype_id"`; Quantity uint32 `json:"quantity"` }
type EquipmentSlotState struct { Slot string `json:"slot"`; ItemArchetypeID string `json:"item_archetype_id"` }
type EquipmentInstanceSlotState struct { Slot string `json:"slot"`; ItemInstanceJSON string `json:"item_instance_json"` }

// InventoryState stays value-comparable because crash replay relies on Snapshot equality. JSON
// strings are canonical Server-internal persistence encodings. The four hand-specific fields are
// legacy read compatibility only; current schema writes EquipmentJSON/EquipmentInstancesJSON.
type InventoryState struct {
	Initialized            bool   `json:"initialized"`
	StacksJSON             string `json:"stacks_json,omitempty"`
	InstancesJSON          string `json:"instances_json,omitempty"`
	EquipmentJSON          string `json:"equipment_json,omitempty"`
	EquipmentInstancesJSON string `json:"equipment_instances_json,omitempty"`
	MainHand               string `json:"main_hand,omitempty"`
	OffHand                string `json:"off_hand,omitempty"`
	MainHandInstanceJSON   string `json:"main_hand_instance_json,omitempty"`
	OffHandInstanceJSON    string `json:"off_hand_instance_json,omitempty"`
}

func NewInventoryStateWithEquipment(stacks []InventoryStack, mainHand, offHand string) (InventoryState, error) {
	return NewInventoryStateWithInstances(stacks, nil, mainHand, offHand, nil, nil)
}

func NewInventoryStateWithInstances(stacks []InventoryStack, instances []iteminstance.Instance, mainHand, offHand string, mainHandInstance, offHandInstance *iteminstance.Instance) (InventoryState, error) {
	equipment := make([]EquipmentSlotState, 0, 2)
	if id := strings.TrimSpace(mainHand); id != "" { equipment = append(equipment, EquipmentSlotState{Slot: "main_hand", ItemArchetypeID: id}) }
	if id := strings.TrimSpace(offHand); id != "" { equipment = append(equipment, EquipmentSlotState{Slot: "off_hand", ItemArchetypeID: id}) }
	equipmentInstances := make([]EquipmentInstanceSlotState, 0, 2)
	if mainHandInstance != nil {
		data, err := iteminstance.CanonicalShapeJSON(*mainHandInstance); if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		equipmentInstances = append(equipmentInstances, EquipmentInstanceSlotState{Slot: "main_hand", ItemInstanceJSON: string(data)})
	}
	if offHandInstance != nil {
		data, err := iteminstance.CanonicalShapeJSON(*offHandInstance); if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		equipmentInstances = append(equipmentInstances, EquipmentInstanceSlotState{Slot: "off_hand", ItemInstanceJSON: string(data)})
	}
	return NewInventoryStateWithSlots(stacks, instances, equipment, equipmentInstances)
}

func NewInventoryStateWithSlots(stacks []InventoryStack, instances []iteminstance.Instance, equipment []EquipmentSlotState, equipmentInstances []EquipmentInstanceSlotState) (InventoryState, error) {
	canonicalStacks := append([]InventoryStack(nil), stacks...)
	for index := range canonicalStacks { canonicalStacks[index].ItemArchetypeID = strings.TrimSpace(canonicalStacks[index].ItemArchetypeID) }
	sort.Slice(canonicalStacks, func(i, j int) bool { return canonicalStacks[i].ItemArchetypeID < canonicalStacks[j].ItemArchetypeID })
	state := InventoryState{Initialized: true}
	if len(canonicalStacks) > 0 { data, err := json.Marshal(canonicalStacks); if err != nil { return InventoryState{}, err }; state.StacksJSON = string(data) }
	canonicalInstances, err := canonicalInstanceList(instances); if err != nil { return InventoryState{}, err }
	state.InstancesJSON, err = encodeCanonicalInstances(canonicalInstances); if err != nil { return InventoryState{}, err }

	canonicalEquipment := append([]EquipmentSlotState(nil), equipment...)
	for index := range canonicalEquipment {
		canonicalEquipment[index].Slot = strings.TrimSpace(canonicalEquipment[index].Slot)
		canonicalEquipment[index].ItemArchetypeID = strings.TrimSpace(canonicalEquipment[index].ItemArchetypeID)
	}
	sort.Slice(canonicalEquipment, func(i, j int) bool { return equipmentSlotRank(canonicalEquipment[i].Slot) < equipmentSlotRank(canonicalEquipment[j].Slot) })
	if len(canonicalEquipment) > 0 { data, err := json.Marshal(canonicalEquipment); if err != nil { return InventoryState{}, err }; state.EquipmentJSON = string(data) }

	canonicalEquippedInstances := append([]EquipmentInstanceSlotState(nil), equipmentInstances...)
	for index := range canonicalEquippedInstances {
		canonicalEquippedInstances[index].Slot = strings.TrimSpace(canonicalEquippedInstances[index].Slot)
		instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(canonicalEquippedInstances[index].ItemInstanceJSON)); if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		data, err := iteminstance.CanonicalShapeJSON(instance); if err != nil { return InventoryState{}, ErrInvalidSnapshot }
		canonicalEquippedInstances[index].ItemInstanceJSON = string(data)
	}
	sort.Slice(canonicalEquippedInstances, func(i, j int) bool { return equipmentSlotRank(canonicalEquippedInstances[i].Slot) < equipmentSlotRank(canonicalEquippedInstances[j].Slot) })
	if len(canonicalEquippedInstances) > 0 { data, err := json.Marshal(canonicalEquippedInstances); if err != nil { return InventoryState{}, err }; state.EquipmentInstancesJSON = string(data) }
	if err := validateInventoryState(state); err != nil { return InventoryState{}, err }
	return state, nil
}

func validEquipmentSlot(slot string) bool { return equipmentSlotRank(slot) < 7 }
func equipmentSlotRank(slot string) int {
	switch slot { case "main_hand": return 0; case "off_hand": return 1; case "helmet": return 2; case "chest": return 3; case "gloves": return 4; case "legs": return 5; case "boots": return 6; default: return 99 }
}

func (state InventoryState) Stacks() ([]InventoryStack, error) {
	if state.StacksJSON == "" { return nil, nil }; var stacks []InventoryStack
	if err := json.Unmarshal([]byte(state.StacksJSON), &stacks); err != nil { return nil, ErrInvalidSnapshot }; return stacks, nil
}
func (state InventoryState) Instances() ([]iteminstance.Instance, error) {
	if state.InstancesJSON == "" { return nil, nil }; var raw []json.RawMessage
	if err := json.Unmarshal([]byte(state.InstancesJSON), &raw); err != nil { return nil, ErrInvalidSnapshot }
	instances := make([]iteminstance.Instance, 0, len(raw)); for _, encoded := range raw { instance, err := iteminstance.DecodeCanonicalShapeJSON(encoded); if err != nil { return nil, ErrInvalidSnapshot }; instances = append(instances, instance) }; return instances, nil
}
func (state InventoryState) Equipment() ([]EquipmentSlotState, error) {
	if state.EquipmentJSON == "" {
		var legacy []EquipmentSlotState
		if state.MainHand != "" { legacy = append(legacy, EquipmentSlotState{Slot:"main_hand", ItemArchetypeID:state.MainHand}) }
		if state.OffHand != "" { legacy = append(legacy, EquipmentSlotState{Slot:"off_hand", ItemArchetypeID:state.OffHand}) }
		return legacy, nil
	}
	var equipment []EquipmentSlotState; if err := json.Unmarshal([]byte(state.EquipmentJSON), &equipment); err != nil { return nil, ErrInvalidSnapshot }; return equipment, nil
}
func (state InventoryState) EquipmentInstances() ([]EquipmentInstanceSlotState, error) {
	if state.EquipmentInstancesJSON == "" {
		var legacy []EquipmentInstanceSlotState
		if state.MainHandInstanceJSON != "" { legacy = append(legacy, EquipmentInstanceSlotState{Slot:"main_hand", ItemInstanceJSON:state.MainHandInstanceJSON}) }
		if state.OffHandInstanceJSON != "" { legacy = append(legacy, EquipmentInstanceSlotState{Slot:"off_hand", ItemInstanceJSON:state.OffHandInstanceJSON}) }
		return legacy, nil
	}
	var equipment []EquipmentInstanceSlotState; if err := json.Unmarshal([]byte(state.EquipmentInstancesJSON), &equipment); err != nil { return nil, ErrInvalidSnapshot }; return equipment, nil
}
func (state InventoryState) MainHandInstance() (iteminstance.Instance, bool, error) { return state.equippedInstance("main_hand") }
func (state InventoryState) OffHandInstance() (iteminstance.Instance, bool, error) { return state.equippedInstance("off_hand") }
func (state InventoryState) equippedInstance(slot string) (iteminstance.Instance, bool, error) {
	items, err := state.EquipmentInstances(); if err != nil { return iteminstance.Instance{}, false, err }
	for _, item := range items { if item.Slot == slot { instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(item.ItemInstanceJSON)); if err != nil { return iteminstance.Instance{}, false, ErrInvalidSnapshot }; return instance, true, nil } }
	return iteminstance.Instance{}, false, nil
}

func canonicalInstanceList(instances []iteminstance.Instance) ([]iteminstance.Instance, error) {
	if len(instances) == 0 { return nil, nil }; canonical := append([]iteminstance.Instance(nil), instances...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })
	for index := range canonical { data, err := iteminstance.CanonicalShapeJSON(canonical[index]); if err != nil { return nil, ErrInvalidSnapshot }; decoded, err := iteminstance.DecodeCanonicalShapeJSON(data); if err != nil { return nil, ErrInvalidSnapshot }; canonical[index] = decoded; if index > 0 && canonical[index-1].ID >= canonical[index].ID { return nil, ErrInvalidSnapshot } }
	return canonical, nil
}
func encodeCanonicalInstances(instances []iteminstance.Instance) (string, error) {
	if len(instances) == 0 { return "", nil }; raw := make([]json.RawMessage, 0, len(instances)); for _, instance := range instances { data, err := iteminstance.CanonicalShapeJSON(instance); if err != nil { return "", ErrInvalidSnapshot }; raw = append(raw, json.RawMessage(data)) }; data, err := json.Marshal(raw); if err != nil { return "", err }; return string(data), nil
}

func validateInventoryState(state InventoryState) error {
	if !state.Initialized {
		if state.StacksJSON != "" || state.InstancesJSON != "" || state.EquipmentJSON != "" || state.EquipmentInstancesJSON != "" || state.MainHand != "" || state.OffHand != "" || state.MainHandInstanceJSON != "" || state.OffHandInstanceJSON != "" { return ErrInvalidSnapshot }; return nil
	}
	if (state.EquipmentJSON != "" || state.EquipmentInstancesJSON != "") && (state.MainHand != "" || state.OffHand != "" || state.MainHandInstanceJSON != "" || state.OffHandInstanceJSON != "") { return ErrInvalidSnapshot }
	if state.MainHand != strings.TrimSpace(state.MainHand) || state.OffHand != strings.TrimSpace(state.OffHand) { return ErrInvalidSnapshot }
	stacks, err := state.Stacks(); if err != nil { return err }; lastStack := ""
	for _, stack := range stacks { id := strings.TrimSpace(stack.ItemArchetypeID); if id == "" || id != stack.ItemArchetypeID || stack.Quantity == 0 || (lastStack != "" && id <= lastStack) { return ErrInvalidSnapshot }; lastStack = id }
	if len(stacks) == 0 && state.StacksJSON != "" { return ErrInvalidSnapshot }; if len(stacks) > 0 { data, err := json.Marshal(stacks); if err != nil || string(data) != state.StacksJSON { return ErrInvalidSnapshot } }
	instances, err := state.Instances(); if err != nil { return err }; lastInstanceID := iteminstance.ID(""); seen := make(map[iteminstance.ID]struct{}, len(instances)+7)
	for _, instance := range instances { if lastInstanceID != "" && instance.ID <= lastInstanceID { return ErrInvalidSnapshot }; lastInstanceID = instance.ID; if _, duplicate := seen[instance.ID]; duplicate { return ErrInvalidSnapshot }; seen[instance.ID] = struct{}{} }
	if len(instances) == 0 && state.InstancesJSON != "" { return ErrInvalidSnapshot }; if len(instances) > 0 { encoded, err := encodeCanonicalInstances(instances); if err != nil || encoded != state.InstancesJSON { return ErrInvalidSnapshot } }
	equipment, err := state.Equipment(); if err != nil { return err }; occupied := make(map[string]struct{}, 7); lastRank := -1
	for _, item := range equipment { rank := equipmentSlotRank(item.Slot); if rank >= 7 || rank <= lastRank || strings.TrimSpace(item.ItemArchetypeID) == "" || item.ItemArchetypeID != strings.TrimSpace(item.ItemArchetypeID) { return ErrInvalidSnapshot }; lastRank = rank; occupied[item.Slot] = struct{}{} }
	if state.EquipmentJSON != "" { data, err := json.Marshal(equipment); if err != nil || string(data) != state.EquipmentJSON { return ErrInvalidSnapshot } }
	equippedInstances, err := state.EquipmentInstances(); if err != nil { return err }; lastRank = -1
	for _, item := range equippedInstances { rank := equipmentSlotRank(item.Slot); if rank >= 7 || rank <= lastRank { return ErrInvalidSnapshot }; lastRank = rank; if _, duplicateSlot := occupied[item.Slot]; duplicateSlot { return ErrInvalidSnapshot }; occupied[item.Slot] = struct{}{}; instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(item.ItemInstanceJSON)); if err != nil { return ErrInvalidSnapshot }; if _, duplicate := seen[instance.ID]; duplicate { return ErrInvalidSnapshot }; seen[instance.ID] = struct{}{}; data, err := iteminstance.CanonicalShapeJSON(instance); if err != nil || string(data) != item.ItemInstanceJSON { return ErrInvalidSnapshot } }
	if state.EquipmentInstancesJSON != "" { data, err := json.Marshal(equippedInstances); if err != nil || string(data) != state.EquipmentInstancesJSON { return ErrInvalidSnapshot } }
	return nil
}

func CanonicalInventoryState(state InventoryState) (InventoryState, error) {
	if !state.Initialized { if err := validateInventoryState(state); err != nil { return InventoryState{}, err }; return InventoryState{}, nil }
	if err := validateInventoryState(state); err != nil { return InventoryState{}, err }
	stacks, err := state.Stacks(); if err != nil { return InventoryState{}, err }; instances, err := state.Instances(); if err != nil { return InventoryState{}, err }; equipment, err := state.Equipment(); if err != nil { return InventoryState{}, err }; equippedInstances, err := state.EquipmentInstances(); if err != nil { return InventoryState{}, err }
	return NewInventoryStateWithSlots(stacks, instances, equipment, equippedInstances)
}
