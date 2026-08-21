package worldruntime

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if id == 0 || sequence == 0 || action.ActionID == "" || action.TargetKind == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	if action.TargetKind == protocol.ActionTargetPoint {
		if action.TargetX == nil || action.TargetZ == nil || !finiteActionCoordinate(*action.TargetX) || !finiteActionCoordinate(*action.TargetZ) {
			return errors.New("worldruntime: invalid point action intent")
		}
	} else if action.TargetID == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	return r.queue.tryPush(useActionCommand{sessionID:id,sequence:sequence,action:action})
}

func (r *Runtime) EnqueueAttackGate(id session.ID, sequence uint32, gateID string) error {
	return r.EnqueueUseAction(id, sequence, protocol.ClientUseAction{ActionID:legacyGateActionID,TargetKind:protocol.ActionTargetGate,TargetID:gateID})
}

func finiteActionCoordinate(value float32) bool {
	f := float64(value)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
