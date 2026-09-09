package protocol

const (
	MessageClientUseItem MessageType = 8
	MessageItemUseResult MessageType = 114
)

// ClientUseItem carries intent only. The Client names stable item identity; the Server resolves
// effect magnitude, resource legality, cooldown policy and inventory consumption.
type ClientUseItem struct {
	ItemArchetypeID string
}

func (ClientUseItem) Type() MessageType { return MessageClientUseItem }

type ItemUseOutcome string

const (
	ItemUseOutcomeUsed     ItemUseOutcome = "used"
	ItemUseOutcomeRejected ItemUseOutcome = "rejected"
)

type ItemUseRejectionReason string

const (
	ItemUseRejectionCooldown       ItemUseRejectionReason = "cooldown"
	ItemUseRejectionResourceFull   ItemUseRejectionReason = "resource_full"
	ItemUseRejectionDefeated       ItemUseRejectionReason = "defeated"
	ItemUseRejectionMissingItem    ItemUseRejectionReason = "missing_item"
	ItemUseRejectionNotUsable      ItemUseRejectionReason = "not_usable"
	ItemUseRejectionServerRejected ItemUseRejectionReason = "server_rejected"
)

// ItemUseResult is source-session-only authoritative feedback for one processed Reliable
// ClientUseItem sequence. InventorySnapshot and EntityVitalsState remain gameplay truth; this
// result exists so presentation can acknowledge use/rejection and show the Server-owned cooldown.
// AppliedAmount is the actual clamped HP/MP delta on success. CooldownReadyTick is authoritative
// when non-zero and must never be extended or shortened by the Client.
type ItemUseResult struct {
	ClientActionSequence uint32
	ItemArchetypeID      string
	Outcome              ItemUseOutcome
	Reason               ItemUseRejectionReason
	AppliedAmount        uint32
	CooldownReadyTick    uint64
}

func (ItemUseResult) Type() MessageType { return MessageItemUseResult }
