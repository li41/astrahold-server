package jsonv1

type itemAffixState struct {
	AffixID  string `json:"affix_id"`
	Strength uint8  `json:"strength"`
	Value    uint32 `json:"value"`
}

type itemInstanceState struct {
	ItemInstanceID  string           `json:"item_instance_id"`
	ItemArchetypeID string           `json:"item_archetype_id"`
	Affixes         []itemAffixState `json:"affixes"`
}

type clientEquipmentInstanceCommand struct {
	Operation      string `json:"operation"`
	Slot           string `json:"slot"`
	ItemInstanceID string `json:"item_instance_id,omitempty"`
}

type inventoryInstanceSnapshot struct {
	Revision uint64              `json:"revision"`
	Items    []itemInstanceState `json:"items"`
}

type equipmentInstanceSlotState struct {
	Slot string            `json:"slot"`
	Item itemInstanceState `json:"item"`
}

type equipmentInstanceSnapshot struct {
	Revision uint64                       `json:"revision"`
	Slots    []equipmentInstanceSlotState `json:"slots"`
}
