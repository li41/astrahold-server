package protocol

const (
	MessageClientWarehouseCommand MessageType = 125
	MessageWarehouseResult        MessageType = 126
	MessageWarehouseSnapshot      MessageType = 127
)

type WarehouseOperation string

const (
	WarehouseOperationOpen     WarehouseOperation = "open"
	WarehouseOperationDeposit  WarehouseOperation = "deposit"
	WarehouseOperationWithdraw WarehouseOperation = "withdraw"
)

// ClientWarehouseCommand carries intent only. Account/character identity, current map,
// interaction range and inventory/warehouse truth are all derived and validated by the Server.
type ClientWarehouseCommand struct {
	Operation       WarehouseOperation
	ItemArchetypeID string
	Quantity        uint32
}

func (ClientWarehouseCommand) Type() MessageType { return MessageClientWarehouseCommand }

type WarehouseOutcome string

const (
	WarehouseOutcomeOpened    WarehouseOutcome = "opened"
	WarehouseOutcomeDeposited WarehouseOutcome = "deposited"
	WarehouseOutcomeWithdrawn WarehouseOutcome = "withdrawn"
	WarehouseOutcomeRejected  WarehouseOutcome = "rejected"
)

type WarehouseRejectionReason string

const (
	WarehouseRejectionNotAuthorized         WarehouseRejectionReason = "not_authorized"
	WarehouseRejectionWrongMap              WarehouseRejectionReason = "wrong_map"
	WarehouseRejectionOutOfRange            WarehouseRejectionReason = "out_of_range"
	WarehouseRejectionDefeated              WarehouseRejectionReason = "defeated"
	WarehouseRejectionEquipmentNotSupported WarehouseRejectionReason = "equipment_not_supported"
	WarehouseRejectionInsufficientInventory WarehouseRejectionReason = "insufficient_inventory"
	WarehouseRejectionInsufficientWarehouse WarehouseRejectionReason = "insufficient_warehouse"
	WarehouseRejectionInventoryRejected     WarehouseRejectionReason = "inventory_rejected"
	WarehouseRejectionServerRejected        WarehouseRejectionReason = "server_rejected"
)

// WarehouseResult correlates one processed Reliable intent. Authoritative contents are supplied by
// WarehouseSnapshot and character inventory snapshots rather than inferred from this result.
type WarehouseResult struct {
	ClientActionSequence uint32
	Operation            WarehouseOperation
	Outcome              WarehouseOutcome
	Reason               WarehouseRejectionReason
	ItemArchetypeID      string
	Quantity             uint32
}

func (WarehouseResult) Type() MessageType { return MessageWarehouseResult }

type WarehouseItemStack struct {
	ItemArchetypeID string
	Quantity        uint32
}

// WarehouseSnapshot is a complete replacement snapshot. An empty Items slice clears stale Client
// presentation state.
type WarehouseSnapshot struct {
	Items []WarehouseItemStack
}

func (WarehouseSnapshot) Type() MessageType { return MessageWarehouseSnapshot }
