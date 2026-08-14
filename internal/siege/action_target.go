package siege

import (
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

// ApplyActionDamage 是 S3-D.2 generic Combat Action 進入 Gate target domain 的入口。
// attackRange 與 damage 由 combat.Service 的 PreparedAction 提供；本方法只做 Gate-specific
// alive / Layer / Range / LOS / HP / blocker transaction，不消耗 action cooldown。
func (s *Service) ApplyActionDamage(position world.Position, gateID string, attackRange float32, damage combat.Damage, scene World) (GateState, error) {
	gate, ok := s.gates[gateID]
	if !ok {
		return GateState{}, ErrUnknownGate
	}
	state := GateState{ID: gateID, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0}
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
	if distanceToBoundsXZ(position.X, position.Z, blocker.Bounds) > attackRange {
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
	if damage.Amount >= newHP {
		newHP = 0
	} else {
		newHP -= damage.Amount
	}
	if newHP == 0 {
		if err := scene.SetBlockerEnabled(gate.definition.BlockerID, false); err != nil {
			return state, err
		}
	}
	gate.hp = newHP
	return GateState{ID: gateID, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0}, nil
}
