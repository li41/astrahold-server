package protocol

import "github.com/li41/astrahold-server/internal/world"

const MessageCharacterClassResourceState MessageType = 117

// CharacterClassResourceState is complete resendable runtime combat-resource truth for one
// character. ResourceID is a stable Server vocabulary; Current and Max are authoritative values.
type CharacterClassResourceState struct {
	EntityID   world.EntityID
	ResourceID string
	Current    uint32
	Max        uint32
}

func (CharacterClassResourceState) Type() MessageType { return MessageCharacterClassResourceState }
