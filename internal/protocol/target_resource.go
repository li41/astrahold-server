package protocol

import "github.com/li41/astrahold-server/internal/world"

const MessageCharacterTargetResourceState MessageType = 118

// CharacterTargetResourceState is complete authoritative state for one resource owned by a
// source character against one target. Current=0 clears that source-target presentation state.
type CharacterTargetResourceState struct {
	SourceEntityID world.EntityID
	TargetEntityID world.EntityID
	ResourceID     string
	Current        uint32
	Max            uint32
}

func (CharacterTargetResourceState) Type() MessageType { return MessageCharacterTargetResourceState }
