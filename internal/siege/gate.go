// Package siege 包含 Astrahold 攻城目標的權威 domain state。
package siege

import (
	"errors"
	"math"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrUnknownGate          = errors.New("siege: unknown gate")
	ErrGateDestroyed        = errors.New("siege: gate destroyed")
	ErrGateWrongLayer       = errors.New("siege: gate wrong layer")
	ErrGateOutOfRange       = errors.New("siege: gate out of range")
	ErrGateNoLineOfSight    = errors.New("siege: gate no line of sight")
	ErrGateAttackCooldown   = errors.New("siege: gate attack cooldown")
	ErrGateBlockerDisabled  = errors.New("siege: gate blocker disabled before destruction")
)

type World interface {
	BlockerDefinition(id string) (gameplayworld.Blocker, error)
	BlockerEnabled(id string) (bool, error)
	SetBlockerEnabled(id string, enabled bool) error
	HasLineOfSightIgnoringBlocker(from, to world.Position, ignoreBlockerID string) bool
}

type GateState struct {
	ID        string
	HP        uint32
	MaxHP     uint32
	Destroyed bool
}

type gateRuntime struct {
	definition gameplayworld.Gate
	hp         uint32
}

type attackKey struct {
	entityID world.EntityID
	gateID   string
}

type Service struct {
	gates          map[string]*gateRuntime
	order          []string
	nextAttackTick map[attackKey]uint64
	match          *matchRuntime
}

func NewService(definitions []gameplayworld.Gate) *Service {
	s := &Service{
		gates:          make(map[string]*gateRuntime, len(definitions)),
		order:          make([]string, 0, len(definitions)),
		nextAttackTick: make(map[attackKey]uint64),
	}
	for _, definition := range definitions {
		copy := definition
		s.gates[definition.ID] = &gateRuntime{definition: copy, hp: definition.MaxHP}
		s.order = append(s.order, definition.ID)
	}
	return s
}

func (s *Service) States() []GateState {
	states := make([]GateState, 0, len(s.order))
	for _, id := range s.order {
		gate := s.gates[id]
		states = append(states, GateState{ID: id, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0})
	}
	return states
}

// Attack 執行最小 S3-D Gate interaction。Client 不提供 damage；所有參數來自 Gameplay World。
// 呼叫者必須是 world simulation owner，確保 Gate HP 與 blocker state 原子更新。
func (s *Service) Attack(entityID world.EntityID, position world.Position, gateID string, tick uint64, delta time.Duration, scene World) (GateState, error) {
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
	if distanceToBoundsXZ(position.X, position.Z, blocker.Bounds) > gate.definition.Attack.Range {
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

	key := attackKey{entityID: entityID, gateID: gateID}
	if next := s.nextAttackTick[key]; next != 0 && tick < next {
		return state, ErrGateAttackCooldown
	}

	newHP := gate.hp
	if gate.definition.Attack.Damage >= newHP {
		newHP = 0
	} else {
		newHP -= gate.definition.Attack.Damage
	}
	if newHP == 0 {
		if err := scene.SetBlockerEnabled(gate.definition.BlockerID, false); err != nil {
			return state, err
		}
	}
	gate.hp = newHP
	s.nextAttackTick[key] = tick + cooldownTicks(gate.definition.Attack.CooldownSeconds, delta)
	return GateState{ID: gateID, HP: gate.hp, MaxHP: gate.definition.MaxHP, Destroyed: gate.hp == 0}, nil
}

func cooldownTicks(seconds float32, delta time.Duration) uint64 {
	if seconds <= 0 || delta <= 0 {
		return 1
	}
	ticks := uint64(math.Ceil(float64(seconds) / delta.Seconds()))
	if ticks == 0 {
		return 1
	}
	return ticks
}

func distanceToBoundsXZ(x, z float32, bounds gameplayworld.BoundsXZ) float32 {
	dx := axisDistance(x, bounds.MinX, bounds.MaxX)
	dz := axisDistance(z, bounds.MinZ, bounds.MaxZ)
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

func axisDistance(value, minValue, maxValue float32) float32 {
	switch {
	case value < minValue:
		return minValue - value
	case value > maxValue:
		return value - maxValue
	default:
		return 0
	}
}

func clamp(value, minValue, maxValue float32) float32 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
