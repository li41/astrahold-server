package protocol

const (
	MessageClientEnhanceEquipment    MessageType = 123
	MessageEquipmentEnhancementResult MessageType = 124
)

// ClientEnhanceEquipment carries intent only. The Client identifies one owned scroll stack and
// one exact unique equipment instance. Compatibility, current enhancement level, outcome and
// inventory consumption are all Server-authoritative.
type ClientEnhanceEquipment struct {
	ScrollItemArchetypeID string
	ItemInstanceID        string
}

func (ClientEnhanceEquipment) Type() MessageType { return MessageClientEnhanceEquipment }

type EquipmentEnhancementOutcome string

const (
	EquipmentEnhancementOutcomeEnhanced EquipmentEnhancementOutcome = "enhanced"
	EquipmentEnhancementOutcomeRejected EquipmentEnhancementOutcome = "rejected"
)

type EquipmentEnhancementRejectionReason string

const (
	EquipmentEnhancementRejectionMissingScroll EquipmentEnhancementRejectionReason = "missing_scroll"
	EquipmentEnhancementRejectionMissingTarget EquipmentEnhancementRejectionReason = "missing_target"
	EquipmentEnhancementRejectionWrongScroll   EquipmentEnhancementRejectionReason = "wrong_scroll"
	EquipmentEnhancementRejectionDefeated      EquipmentEnhancementRejectionReason = "defeated"
	EquipmentEnhancementRejectionAtLimit       EquipmentEnhancementRejectionReason = "at_limit"
	EquipmentEnhancementRejectionServerRejected EquipmentEnhancementRejectionReason = "server_rejected"
)

// EquipmentEnhancementResult is source-session feedback for one processed Reliable intent.
// Item-instance and inventory snapshots remain gameplay truth.
type EquipmentEnhancementResult struct {
	ClientActionSequence  uint32
	ScrollItemArchetypeID string
	ItemInstanceID        string
	Outcome               EquipmentEnhancementOutcome
	Reason                EquipmentEnhancementRejectionReason
	PreviousLevel         uint16
	CurrentLevel          uint16
	ScrollConsumed        bool
}

func (EquipmentEnhancementResult) Type() MessageType { return MessageEquipmentEnhancementResult }
