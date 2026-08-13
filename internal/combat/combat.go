// Package combat 定義 Astrahold 可重用的權威 Action / Damage source。
package combat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/li41/astrahold-server/internal/world"
)

const SchemaVersion uint16 = 1

type TargetKind string

const (
	TargetGate TargetKind = "gate"
)

type DamageType string

const (
	DamagePhysical DamageType = "physical"
)

var (
	ErrUnsupportedSchema = errors.New("combat: unsupported schema version")
	ErrInvalidDefinition = errors.New("combat: invalid definition")
	ErrUnknownAction      = errors.New("combat: unknown action")
	ErrTargetNotAllowed   = errors.New("combat: target not allowed")
	ErrActionCooldown     = errors.New("combat: action cooldown")
)

type ActionDefinition struct {
	ID              string       `json:"id"`
	Targets         []TargetKind `json:"targets"`
	Range           float32      `json:"range"`
	BaseDamage      uint32       `json:"base_damage"`
	DamageType      DamageType   `json:"damage_type"`
	CooldownSeconds float32      `json:"cooldown_seconds"`
}

type Definition struct {
	SchemaVersion uint16             `json:"schema_version"`
	Revision      string             `json:"revision"`
	Actions       []ActionDefinition `json:"actions"`
}

type Loaded struct {
	Definition Definition
}

type DamageSource struct {
	ActorEntityID world.EntityID
	ActionID      string
}

type Damage struct {
	Source DamageSource
	Type   DamageType
	Amount uint32
}

type Target struct {
	Kind TargetKind
	ID   string
}

type PreparedAction struct {
	ActorEntityID world.EntityID
	Definition    ActionDefinition
	Target        Target
	Damage        Damage
}

type cooldownKey struct {
	entityID world.EntityID
	actionID string
}

type Service struct {
	actions     map[string]ActionDefinition
	nextUseTick map[cooldownKey]uint64
}

func LoadFile(path string) (Loaded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	return Load(bytes.NewReader(data))
}

func Load(r io.Reader) (Loaded, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var definition Definition
	if err := decoder.Decode(&definition); err != nil {
		return Loaded{}, fmt.Errorf("%w: decode: %v", ErrInvalidDefinition, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Loaded{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidDefinition)
		}
		return Loaded{}, fmt.Errorf("%w: trailing data: %v", ErrInvalidDefinition, err)
	}
	if err := Validate(definition); err != nil {
		return Loaded{}, err
	}
	return Loaded{Definition: definition}, nil
}

func Validate(definition Definition) error {
	if definition.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, definition.SchemaVersion, SchemaVersion)
	}
	if definition.Revision == "" || len(definition.Actions) == 0 {
		return fmt.Errorf("%w: revision/actions", ErrInvalidDefinition)
	}
	ids := make(map[string]struct{}, len(definition.Actions))
	for i, action := range definition.Actions {
		if action.ID == "" || !positiveFinite(action.Range) || action.BaseDamage == 0 || !positiveFinite(action.CooldownSeconds) || !validDamageType(action.DamageType) || len(action.Targets) == 0 {
			return fmt.Errorf("%w: action[%d]", ErrInvalidDefinition, i)
		}
		if _, exists := ids[action.ID]; exists {
			return fmt.Errorf("%w: duplicate action id %q", ErrInvalidDefinition, action.ID)
		}
		ids[action.ID] = struct{}{}

		targets := make(map[TargetKind]struct{}, len(action.Targets))
		for _, target := range action.Targets {
			if !validTargetKind(target) {
				return fmt.Errorf("%w: action %q target %q", ErrInvalidDefinition, action.ID, target)
			}
			if _, exists := targets[target]; exists {
				return fmt.Errorf("%w: action %q duplicate target %q", ErrInvalidDefinition, action.ID, target)
			}
			targets[target] = struct{}{}
		}
	}
	return nil
}

func NewService(definitions []ActionDefinition) (*Service, error) {
	definition := Definition{SchemaVersion: SchemaVersion, Revision: "runtime", Actions: definitions}
	if err := Validate(definition); err != nil {
		return nil, err
	}
	actions := make(map[string]ActionDefinition, len(definitions))
	for _, action := range definitions {
		copy := action
		copy.Targets = append([]TargetKind(nil), action.Targets...)
		actions[action.ID] = copy
	}
	return &Service{actions: actions, nextUseTick: make(map[cooldownKey]uint64)}, nil
}

// Prepare 驗證 action catalog、target capability 與 cooldown，但不消耗 cooldown。
// Target-specific range / LOS / alive validation 由對應 domain 完成；成功套用後才 Commit。
func (s *Service) Prepare(actorEntityID world.EntityID, actionID string, target Target, tick uint64) (PreparedAction, error) {
	action, ok := s.actions[actionID]
	if !ok {
		return PreparedAction{}, ErrUnknownAction
	}
	if target.ID == "" || !containsTarget(action.Targets, target.Kind) {
		return PreparedAction{}, ErrTargetNotAllowed
	}
	key := cooldownKey{entityID: actorEntityID, actionID: actionID}
	if next := s.nextUseTick[key]; next != 0 && tick < next {
		return PreparedAction{}, ErrActionCooldown
	}
	return PreparedAction{
		ActorEntityID: actorEntityID,
		Definition:    action,
		Target:        target,
		Damage: Damage{
			Source: DamageSource{ActorEntityID: actorEntityID, ActionID: actionID},
			Type:   action.DamageType,
			Amount: action.BaseDamage,
		},
	}, nil
}

// Commit 只可在 target domain 已成功套用 action 後呼叫。
func (s *Service) Commit(action PreparedAction, tick uint64, delta time.Duration) {
	key := cooldownKey{entityID: action.ActorEntityID, actionID: action.Definition.ID}
	s.nextUseTick[key] = tick + cooldownTicks(action.Definition.CooldownSeconds, delta)
}

func containsTarget(targets []TargetKind, target TargetKind) bool {
	for _, candidate := range targets {
		if candidate == target {
			return true
		}
	}
	return false
}

func validTargetKind(kind TargetKind) bool {
	switch kind {
	case TargetGate:
		return true
	default:
		return false
	}
}

func validDamageType(kind DamageType) bool {
	switch kind {
	case DamagePhysical:
		return true
	default:
		return false
	}
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

func positiveFinite(value float32) bool {
	f := float64(value)
	return value > 0 && !math.IsNaN(f) && !math.IsInf(f, 0)
}
