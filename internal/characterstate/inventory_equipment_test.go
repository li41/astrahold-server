package characterstate

import (
	"encoding/json"
	"testing"
)

func TestInventoryStatePersistsGenericEquipmentSlotsCanonically(t *testing.T) {
	state, err := NewInventoryStateWithSlots(
		nil,
		nil,
		[]EquipmentSlotState{
			{Slot: "boots", ItemArchetypeID: "item_low_cloth_boots"},
			{Slot: "helmet", ItemArchetypeID: "item_low_cloth_helmet"},
			{Slot: "main_hand", ItemArchetypeID: "item_training_blade"},
		},
		nil,
	)
	if err != nil { t.Fatal(err) }
	var got []EquipmentSlotState
	if err := json.Unmarshal([]byte(state.EquipmentJSON), &got); err != nil { t.Fatal(err) }
	if len(got) != 3 || got[0].Slot != "main_hand" || got[1].Slot != "helmet" || got[2].Slot != "boots" {
		t.Fatalf("canonical equipment=%#v", got)
	}
	if state.MainHand != "" || state.OffHand != "" { t.Fatalf("new state wrote legacy hand fields: %#v", state) }
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
