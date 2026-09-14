package protocol

import "github.com/li41/astrahold-server/internal/world"

const MessageCharacterClassResourceState MessageType = 117

// CharacterClassResourceState is a legacy class-named compatibility lane retained in Protocol v28.
// It still carries complete resendable Server-authoritative runtime combat-resource truth for one
// character; ResourceID is stable Server vocabulary, and Current/Max are authoritative values.
// The type name must not be interpreted as restoring fixed-class gameplay authority.
type CharacterClassResourceState struct {
	EntityID   world.EntityID
	ResourceID string
	Current    uint32
	Max        uint32
}

func (CharacterClassResourceState) Type() MessageType { return MessageCharacterClassResourceState }
