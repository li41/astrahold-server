package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if id == 0 || sequence == 0 || action.ActionID == "" || action.TargetKind == "" || action.TargetID == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	return r.queue.tryPush(useActionCommand{sessionID:id,sequence:sequence,action:action})
}

func (r *Runtime) EnqueueAttackGate(id session.ID, sequence uint32, gateID string) error {
	return r.EnqueueUseAction(id, sequence, protocol.ClientUseAction{ActionID:legacyGateActionID,TargetKind:protocol.ActionTargetGate,TargetID:gateID})
}
