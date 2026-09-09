package jsonv1

type itemUseResult struct {
	ClientActionSequence uint32 `json:"client_action_sequence"`
	ItemArchetypeID      string `json:"item_archetype_id"`
	Outcome              string `json:"outcome"`
	Reason               string `json:"reason,omitempty"`
	AppliedAmount        uint32 `json:"applied_amount,omitempty"`
	CooldownReadyTick    uint64 `json:"cooldown_ready_tick,omitempty"`
}
