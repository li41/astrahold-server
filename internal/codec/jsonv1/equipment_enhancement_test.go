package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestEquipmentEnhancementIntentRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ClientEnhanceEquipment{ScrollItemArchetypeID: "item_astrahold_weapon_enhancement_scroll", ItemInstanceID: "instance:weapon"}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	message, err := codec.Unmarshal(protocol.MessageClientEnhanceEquipment, data)
	if err != nil { t.Fatal(err) }
	got, ok := message.(protocol.ClientEnhanceEquipment)
	if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v", message) }
}

func TestEquipmentEnhancementResultRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.EquipmentEnhancementResult{ClientActionSequence: 9, ScrollItemArchetypeID: "item_astrahold_armor_enhancement_scroll", ItemInstanceID: "instance:armor", Outcome: protocol.EquipmentEnhancementOutcomeEnhanced, PreviousLevel: 2, CurrentLevel: 3, ScrollConsumed: true}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	message, err := codec.Unmarshal(protocol.MessageEquipmentEnhancementResult, data)
	if err != nil { t.Fatal(err) }
	got, ok := message.(protocol.EquipmentEnhancementResult)
	if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v", message) }
}

func TestItemInstanceEnhancementLevelRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.InventoryInstanceSnapshot{Revision: 4, Items: []protocol.ItemInstanceState{{ItemInstanceID: "instance:weapon", ItemArchetypeID: "item_weapon", EnhancementLevel: 6, Affixes: []protocol.ItemAffixState{}}}}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	message, err := codec.Unmarshal(protocol.MessageInventoryInstanceSnapshot, data)
	if err != nil { t.Fatal(err) }
	got, ok := message.(protocol.InventoryInstanceSnapshot)
	if !ok || !reflect.DeepEqual(got, want) { t.Fatalf("got=%#v", message) }
}
