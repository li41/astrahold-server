package jsonv1

type characterTargetResourceState struct {
	SourceEntityID uint64 `json:"source_entity_id"`
	TargetEntityID uint64 `json:"target_entity_id"`
	ResourceID     string `json:"resource_id"`
	Current        uint32 `json:"current"`
	Max            uint32 `json:"max"`
}
