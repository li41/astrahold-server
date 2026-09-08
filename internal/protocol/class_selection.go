package protocol

const (
	MessageClientInitialClassSelection   MessageType = 9
	MessageCharacterClassState           MessageType = 115
	MessageInitialClassSelectionResult   MessageType = 116
)

// ClientInitialClassSelection is an intent only. The Server validates the canonical ClassID,
// ownership, equipment legality and durability before changing gameplay truth.
type ClientInitialClassSelection struct {
	ClassID string
}

func (ClientInitialClassSelection) Type() MessageType { return MessageClientInitialClassSelection }

// CharacterClassState is the authoritative profession identity for the connected character.
// Empty ClassID means the character is still unassigned; it is not a seventh profession.
type CharacterClassState struct {
	ClassID string
}

func (CharacterClassState) Type() MessageType { return MessageCharacterClassState }

type InitialClassSelectionOutcome string

const (
	InitialClassSelectionCommitted InitialClassSelectionOutcome = "committed"
	InitialClassSelectionRejected  InitialClassSelectionOutcome = "rejected"
)

type InitialClassSelectionRejectionReason string

const (
	InitialClassSelectionInvalidClass            InitialClassSelectionRejectionReason = "invalid_class"
	InitialClassSelectionAlreadyAssigned         InitialClassSelectionRejectionReason = "already_assigned"
	InitialClassSelectionAssignmentPending       InitialClassSelectionRejectionReason = "assignment_pending"
	InitialClassSelectionEquipmentIllegal        InitialClassSelectionRejectionReason = "equipment_illegal"
	InitialClassSelectionPersistenceUnavailable  InitialClassSelectionRejectionReason = "persistence_unavailable"
	InitialClassSelectionTrustedIdentityRequired InitialClassSelectionRejectionReason = "trusted_identity_required"
	InitialClassSelectionServerRejected          InitialClassSelectionRejectionReason = "server_rejected"
)

// InitialClassSelectionResult correlates one client intent with the authoritative decision.
// ClassID is the authoritative live ClassID after that decision; it is empty while unassigned.
// A committed result is emitted only after durable checkpoint advancement and world-owner commit.
type InitialClassSelectionResult struct {
	ClientActionSequence uint32
	ClassID              string
	Outcome              InitialClassSelectionOutcome
	Reason               InitialClassSelectionRejectionReason
}

func (InitialClassSelectionResult) Type() MessageType { return MessageInitialClassSelectionResult }
