package jsonv1

type clientEnhanceEquipment struct {
	ScrollItemArchetypeID string `json:"scroll_item_archetype_id"`
	ItemInstanceID        string `json:"item_instance_id"`
}

type equipmentEnhancementResult struct {
	ClientActionSequence  uint32 `json:"client_action_sequence"`
	ScrollItemArchetypeID string `json:"scroll_item_archetype_id"`
	ItemInstanceID        string `json:"item_instance_id"`
	Outcome               string `json:"outcome"`
	Reason                string `json:"reason,omitempty"`
	PreviousLevel         uint16 `json:"previous_level"`
	CurrentLevel          uint16 `json:"current_level"`
	ScrollConsumed        bool   `json:"scroll_consumed"`
}
