package protocol

const (
	MessageClientInitialClassSelection MessageType = 9
	MessageCharacterClassState         MessageType = 115
	MessageInitialClassSelectionResult MessageType = 116
)

// ClientInitialClassSelection is retained only as a Protocol v27 compatibility wire type.
// Production ingress no longer accepts fixed-class selection as a gameplay command; decoding
// this message must not be interpreted as permission to mutate authoritative character truth.
type ClientInitialClassSelection struct {
	ClassID string
}

func (ClientInitialClassSelection) Type() MessageType { return MessageClientInitialClassSelection }

// CharacterClassState is retained only as a Protocol v27 compatibility wire type.
// Current classless runtime does not publish this message as authoritative profession state.
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

// InitialClassSelectionResult is retained only as a Protocol v27 compatibility wire type.
// Current production runtime does not commit fixed-class selection or emit a new durable
// profession decision through this message. Outcome/reason constants remain source-compatible
// until the coordinated breaking Protocol cleanup after consumer audit.
type InitialClassSelectionResult struct {
	ClientActionSequence uint32
	ClassID              string
	Outcome              InitialClassSelectionOutcome
	Reason               InitialClassSelectionRejectionReason
}

func (InitialClassSelectionResult) Type() MessageType { return MessageInitialClassSelectionResult }
