package protocol

import "github.com/li41/astrahold-server/internal/world"

const (
	MessageClientInteractNPC MessageType = 5
	MessageNPCInteraction    MessageType = 112
)

// ClientInteractNPC is intent only. The Server re-resolves the target entity and validates
// authoritative position/layer before producing any interaction result.
type ClientInteractNPC struct {
	NPCEntityID world.EntityID
}

func (ClientInteractNPC) Type() MessageType { return MessageClientInteractNPC }

// NPCInteraction is source-session-only authoritative interaction presentation.
// It contains authored dialogue identity/content, but no Shop/Quest transaction semantics.
type NPCInteraction struct {
	NPCEntityID    world.EntityID
	NPCArchetypeID string
	DisplayName    string
	Text           string
}

func (NPCInteraction) Type() MessageType { return MessageNPCInteraction }
