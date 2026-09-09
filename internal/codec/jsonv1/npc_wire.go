package jsonv1

type clientInteractNPC struct {
	NPCEntityID uint64 `json:"npc_entity_id"`
}

type npcInteraction struct {
	NPCEntityID    uint64 `json:"npc_entity_id"`
	NPCArchetypeID string `json:"npc_archetype_id"`
	DisplayName    string `json:"display_name"`
	Text           string `json:"text"`
}
