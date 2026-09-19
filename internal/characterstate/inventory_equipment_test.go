package characterstate

import (
	"encoding/json"
	"testing"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestInventoryStatePersistsGenericEquipmentSlotsCanonically(t *testing.T) {
	state, err := NewInventoryStateWithSlots(
		nil,
		nil,
		[]EquipmentSlotState{
			{Slot: "belt", ItemArchetypeID: "item_test_belt"},
			{Slot: "ring_2", ItemArchetypeID: "item_test_ring_b"},
			{Slot: "boots", ItemArchetypeID: "item_low_cloth_boots"},
			{Slot: "ring_1", ItemArchetypeID: "item_test_ring_a"},
			{Slot: "helmet", ItemArchetypeID: "item_low_cloth_helmet"},
			{Slot: "necklace", ItemArchetypeID: "item_test_necklace"},
			{Slot: "main_hand", ItemArchetypeID: "item_training_blade"},
		},
		nil,
	)
	if err != nil { t.Fatal(err) }
	var got []EquipmentSlotState
	if err := json.Unmarshal([]byte(state.EquipmentJSON), &got); err != nil { t.Fatal(err) }
	wantSlots := []string{"main_hand", "helmet", "boots", "necklace", "ring_1", "ring_2", "belt"}
	if len(got) != len(wantSlots) { t.Fatalf("canonical equipment=%#v", got) }
	for i, want := range wantSlots {
		if got[i].Slot != want { t.Fatalf("canonical equipment[%d]=%#v want slot %q", i, got[i], want) }
	}
	if state.MainHand != "" || state.OffHand != "" { t.Fatalf("new state wrote legacy hand fields: %#v", state) }
}

func TestInventoryStateRoundTripsDistinctAccessoryInstanceSlots(t *testing.T) {
	instances := []iteminstance.Instance{
		{ID: "item-instance:necklace", ItemArchetypeID: "item_test_necklace"},
		{ID: "item-instance:ring-a", ItemArchetypeID: "item_test_ring"},
		{ID: "item-instance:ring-b", ItemArchetypeID: "item_test_ring"},
		{ID: "item-instance:belt", ItemArchetypeID: "item_test_belt"},
	}
	equipped := make([]EquipmentInstanceSlotState, 0, len(instances))
	for i, slot := range []string{"necklace", "ring_1", "ring_2", "belt"} {
		data, err := iteminstance.CanonicalShapeJSON(instances[i])
		if err != nil { t.Fatal(err) }
		equipped = append(equipped, EquipmentInstanceSlotState{Slot: slot, ItemInstanceJSON: string(data)})
	}
	state, err := NewInventoryStateWithSlots(nil, nil, nil, equipped)
	if err != nil { t.Fatal(err) }
	canonical, err := CanonicalInventoryState(state)
	if err != nil { t.Fatal(err) }
	if canonical != state { t.Fatalf("canonical accessory state changed: got=%#v want=%#v", canonical, state) }
	got, err := canonical.EquipmentInstances()
	if err != nil { t.Fatal(err) }
	wantSlots := []string{"necklace", "ring_1", "ring_2", "belt"}
	if len(got) != len(wantSlots) { t.Fatalf("equipped instances=%#v", got) }
	for i, want := range wantSlots {
		if got[i].Slot != want { t.Fatalf("equipped instances[%d].slot=%q want=%q", i, got[i].Slot, want) }
	}
}

func TestCanonicalInventoryStateMigratesLegacyHands(t *testing.T) {
	legacy := InventoryState{Initialized: true, MainHand: "item_training_blade", OffHand: "item_low_shield"}
	canonical, err := CanonicalInventoryState(legacy)
	if err != nil { t.Fatal(err) }
	if canonical.MainHand != "" || canonical.OffHand != "" { t.Fatalf("legacy fields survived canonicalization: %#v", canonical) }
	equipment, err := canonical.Equipment()
	if err != nil { t.Fatal(err) }
	if len(equipment) != 2 || equipment[0].Slot != "main_hand" || equipment[1].Slot != "off_hand" { t.Fatalf("equipment=%#v", equipment) }
}

func TestInventoryStateRejectsDuplicateEquipmentSlot(t *testing.T) {
	_, err := NewInventoryStateWithSlots(nil, nil, []EquipmentSlotState{
		{Slot: "helmet", ItemArchetypeID: "a"},
		{Slot: "helmet", ItemArchetypeID: "b"},
	}, nil)
	if err == nil { t.Fatal("expected duplicate slot rejection") }
}
