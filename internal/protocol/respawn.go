package protocol

const MessageClientRespawnRequest MessageType = 7

// ClientRespawnRequest is intent only. It deliberately carries no entity, spawn point,
// position or timing. The Server resolves the owning defeated character and the respawn
// outcome already bound by authoritative respawn policy.
type ClientRespawnRequest struct{}

func (ClientRespawnRequest) Type() MessageType { return MessageClientRespawnRequest }
