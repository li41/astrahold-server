package jsonv1

type clientShopCommand struct {
	Operation   string `json:"operation"`
	NPCEntityID uint64 `json:"npc_entity_id"`
	OfferID     string `json:"offer_id,omitempty"`
}

type shopOffer struct {
	OfferID         string `json:"offer_id"`
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
	CostArchetypeID string `json:"cost_archetype_id"`
	CostQuantity    uint32 `json:"cost_quantity"`
}

type shopSnapshot struct {
	Revision    string      `json:"revision"`
	NPCEntityID uint64      `json:"npc_entity_id"`
	ShopID      string      `json:"shop_id"`
	Offers      []shopOffer `json:"offers"`
}
