package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestV28UniqueItemAndAppearanceMessageIDsDoNotOverlap(t *testing.T) {
	if protocol.Version != 28 {
		t.Fatalf("protocol version=%d want=28", protocol.Version)
	}
	if protocol.MessageClientEquipmentInstanceCommand != 119 || protocol.MessageInventoryInstanceSnapshot != 120 || protocol.MessageEquipmentInstanceSnapshot != 121 {
		t.Fatalf("unexpected unique-instance message ids: equip=%d inventory=%d equipment=%d",
			protocol.MessageClientEquipmentInstanceCommand,
			protocol.MessageInventoryInstanceSnapshot,
			protocol.MessageEquipmentInstanceSnapshot,
		)
	}
	if protocol.MessageAppearanceSnapshot != 122 {
		t.Fatalf("appearance message id=%d want=122", protocol.MessageAppearanceSnapshot)
	}
}

func TestUniqueItemInstanceMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	instance := protocol.ItemInstanceState{
		ItemInstanceID:  "item-instance:high-sword-1",
		ItemArchetypeID: "item_high_test_sword",
		Affixes: []protocol.ItemAffixState{
			{AffixID: "affix_critical_rating", Strength: 3, Value: 3},
			{AffixID: "affix_strength", Strength: 1, Value: 1},
		},
	}
	messages := []protocol.Message{
		protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemInstanceID: instance.ItemInstanceID},
		protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand},
		protocol.InventoryInstanceSnapshot{Revision: 7, Items: []protocol.ItemInstanceState{instance}},
		protocol.EquipmentInstanceSnapshot{Revision: 9, Slots: []protocol.EquipmentInstanceSlotState{{Slot: protocol.EquipmentSlotMainHand, Item: instance}}},
	}
	for _, message := range messages {
		encoded, err := codec.Marshal(message)
		if err != nil { t.Fatalf("Marshal(%T): %v", message, err) }
		decoded, err := codec.Unmarshal(message.Type(), encoded)
		if err != nil { t.Fatalf("Unmarshal(%T): %v", message, err) }
		if !reflect.DeepEqual(decoded, message) {
			t.Fatalf("round trip %T changed:\n got=%#v\nwant=%#v\njson=%s", message, decoded, message, encoded)
		}
	}
}

func TestUniqueItemInstanceJSONRejectsUnknownFields(t *testing.T) {
	codec := Codec{}
	if _, err := codec.Unmarshal(protocol.MessageClientEquipmentInstanceCommand, []byte(`{"operation":"equip","slot":"main_hand","item_instance_id":"i1","item_archetype_id":"must-not-be-client-authored"}`)); err == nil {
		t.Fatal("client unique-equipment command accepted unknown archetype field")
	}
	if _, err := codec.Unmarshal(protocol.MessageInventoryInstanceSnapshot, []byte(`{"revision":1,"items":[{"item_instance_id":"i1","item_archetype_id":"a1","affixes":[],"mesh":"client-only"}]}`)); err == nil {
		t.Fatal("unique inventory snapshot accepted unknown presentation field")
	}
}
