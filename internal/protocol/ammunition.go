package protocol

const (
	MessageClientAmmunitionCommand MessageType = 128
	MessageAmmunitionResult        MessageType = 129
	MessageAmmunitionState         MessageType = 130
)

type AmmunitionOperation string

const (
	AmmunitionOperationSelect AmmunitionOperation = "select"
)

// ClientAmmunitionCommand carries selection intent only. Inventory ownership, equipped weapon,
// ammunition legality and the actual arrow consumed by a shot remain Server-authoritative.
type ClientAmmunitionCommand struct {
	Operation       AmmunitionOperation
	ItemArchetypeID string
}

func (ClientAmmunitionCommand) Type() MessageType { return MessageClientAmmunitionCommand }

type AmmunitionOutcome string

const (
	AmmunitionOutcomeSelected AmmunitionOutcome = "selected"
	AmmunitionOutcomeRejected AmmunitionOutcome = "rejected"
)

type AmmunitionRejectionReason string

const (
	AmmunitionRejectionInvalidAmmunition    AmmunitionRejectionReason = "invalid_ammunition"
	AmmunitionRejectionInsufficientInventory AmmunitionRejectionReason = "insufficient_inventory"
	AmmunitionRejectionDefeated              AmmunitionRejectionReason = "defeated"
	AmmunitionRejectionServerRejected        AmmunitionRejectionReason = "server_rejected"
)

// AmmunitionResult correlates one processed Reliable selection intent. The Client must not
// optimistically switch gameplay ammunition before receiving Server feedback/state.
type AmmunitionResult struct {
	ClientActionSequence uint32
	Operation            AmmunitionOperation
	Outcome              AmmunitionOutcome
	Reason               AmmunitionRejectionReason
	ItemArchetypeID      string
}

func (AmmunitionResult) Type() MessageType { return MessageAmmunitionResult }

// AmmunitionState is the authoritative current manual preference.
// Empty SelectedItemArchetypeID means automatic selection: wood first, then silver.
// V1 selection is session-scoped and returns to automatic mode on reconnect.
type AmmunitionState struct {
	SelectedItemArchetypeID string
}

func (AmmunitionState) Type() MessageType { return MessageAmmunitionState }
