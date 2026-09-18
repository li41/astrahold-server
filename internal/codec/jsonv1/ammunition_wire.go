package jsonv1

type clientAmmunitionCommand struct {
	Operation       string `json:"operation"`
	ItemArchetypeID string `json:"item_archetype_id"`
}

type ammunitionResult struct {
	ClientActionSequence uint32 `json:"client_action_sequence"`
	Operation            string `json:"operation"`
	Outcome              string `json:"outcome"`
	Reason               string `json:"reason,omitempty"`
	ItemArchetypeID      string `json:"item_archetype_id"`
}

type ammunitionState struct {
	SelectedItemArchetypeID string `json:"selected_item_archetype_id,omitempty"`
}
