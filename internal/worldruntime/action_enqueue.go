package worldruntime

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if id == 0 || sequence == 0 {
		return errors.New("worldruntime: invalid action intent")
	}
	if err := validateActionIntent(action); err != nil {
		return err
	}
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, action: action})
}

func validateActionIntent(action protocol.ClientUseAction) error {
	if action.ActionID == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	switch action.TargetKind {
	case protocol.ActionTargetPoint:
		if action.TargetID != "" || action.TargetX == nil || action.TargetZ == nil || !finiteActionCoordinate(*action.TargetX) || !finiteActionCoordinate(*action.TargetZ) {
			return errors.New("worldruntime: invalid point action intent")
		}
		return nil
	case protocol.ActionTargetEntity, protocol.ActionTargetGate:
		if action.TargetID == "" || action.TargetX != nil || action.TargetZ != nil {
			return errors.New("worldruntime: invalid action intent")
		}
		return nil
	default:
		return errors.New("worldruntime: invalid action intent")
	}
}

func (r *Runtime) EnqueueAttackGate(id session.ID, sequence uint32, gateID string) error {
	return r.EnqueueUseAction(id, sequence, protocol.ClientUseAction{ActionID: legacyGateActionID, TargetKind: protocol.ActionTargetGate, TargetID: gateID})
}

func finiteActionCoordinate(value float32) bool {
	f := float64(value)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
