package siege

import (
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

// ApplyPreparedAction 將已由 Combat Catalog Prepare 的 action 套用到 Gate target。
// Combat 決定 range / damage / damage type / cooldown；Siege 只負責 Gate-specific validation 與 HP/blocker transaction。
func (s *Service) ApplyPreparedAction(position world.Position, action combat.PreparedAction, scene World) (GateState, error) {
	if action.Target.Kind != combat.TargetGate {
		return GateState{}, combat.ErrTargetNotAllowed
	}
	gate, ok := s.gates[action.Target.ID]
	if !ok {
		return GateState{}, ErrUnknownGate
	}
	state := GateState{ID: action.Target.ID, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0}
	if state.Destroyed {
		return state, ErrGateDestroyed
	}

	blocker, err := scene.BlockerDefinition(gate.definition.BlockerID)
	if err != nil {
		return state, err
	}
	if position.Layer != blocker.Layer {
		return state, ErrGateWrongLayer
	}
	if distanceToBoundsXZ(position.X, position.Z, blocker.Bounds) > action.Definition.Range {
		return state, ErrGateOutOfRange
	}

	enabled, err := scene.BlockerEnabled(gate.definition.BlockerID)
	if err != nil {
		return state, err
	}
	if !enabled {
		return state, ErrGateBlockerDisabled
	}

	target := world.Position{
		X:     clamp(position.X, blocker.Bounds.MinX, blocker.Bounds.MaxX),
		Y:     clamp(position.Y, blocker.MinY, blocker.MaxY),
		Z:     clamp(position.Z, blocker.Bounds.MinZ, blocker.Bounds.MaxZ),
		Layer: blocker.Layer,
	}
	if !scene.HasLineOfSightIgnoringBlocker(position, target, gate.definition.BlockerID) {
		return state, ErrGateNoLineOfSight
	}

	newHP := gate.hp
	if action.Damage.Amount >= newHP {
		newHP = 0
	} else {
		newHP -= action.Damage.Amount
	}
	if newHP == 0 {
		if err := scene.SetBlockerEnabled(gate.definition.BlockerID, false); err != nil {
			return state, err
		}
	}
	gate.hp = newHP
	return GateState{ID: action.Target.ID, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0}, nil
}
