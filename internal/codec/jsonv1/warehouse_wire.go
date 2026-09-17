package jsonv1

type clientWarehouseCommand struct {
	Operation       string `json:"operation"`
	ItemArchetypeID string `json:"item_archetype_id,omitempty"`
	Quantity        uint32 `json:"quantity,omitempty"`
}

type warehouseResult struct {
	ClientActionSequence uint32 `json:"client_action_sequence"`
	Operation            string `json:"operation"`
	Outcome              string `json:"outcome"`
	Reason               string `json:"reason,omitempty"`
	ItemArchetypeID      string `json:"item_archetype_id,omitempty"`
	Quantity             uint32 `json:"quantity,omitempty"`
}

type warehouseItemStack struct {
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
}

type warehouseSnapshot struct {
	Items []warehouseItemStack `json:"items"`
}
