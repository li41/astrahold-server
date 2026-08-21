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

const SchemaVersion uint16 = 2

type TargetKind string

const (
	TargetGate   TargetKind = "gate"
	TargetEntity TargetKind = "entity"
	TargetPoint  TargetKind = "point"
)

type ActionEffect string

const (
	EffectDamage    ActionEffect = "damage"
	EffectResurrect ActionEffect = "resurrect"
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
	Effect          ActionEffect `json:"effect,omitempty"`
	Targets         []TargetKind `json:"targets"`
	Range           float32      `json:"range"`
	HitRadius       float32      `json:"hit_radius,omitempty"`
	BaseDamage      uint32       `json:"base_damage,omitempty"`
	DamageType      DamageType   `json:"damage_type,omitempty"`
	ReviveHPPercent uint8        `json:"revive_hp_percent,omitempty"`
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
	Kind     TargetKind
	ID       string
	PointX   float32
	PointZ   float32
	HasPoint bool
}

// Intent is the transport-neutral combat request consumed after ingress/session/AI validation.
// It deliberately contains ActorEntityID rather than a network SessionID.
type Intent struct {
	ActorEntityID world.EntityID
	ActionID      string
	Target        Target
}

type PreparedAction struct {
	ActionInstanceID uint64
	ActorEntityID    world.EntityID
	Definition       ActionDefinition
	Target           Target
	Damage           Damage
}

type cooldownKey struct {
	entityID world.EntityID
	actionID string
}

type Service struct {
	actions        map[string]ActionDefinition
	nextUseTick    map[cooldownKey]uint64
	nextInstanceID uint64
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
	for i := range definition.Actions {
		definition.Actions[i] = normalizeAction(definition.Actions[i])
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
		if action.ID == "" || !positiveFinite(action.Range) || !positiveFinite(action.CooldownSeconds) || len(action.Targets) == 0 {
			return fmt.Errorf("%w: action[%d]", ErrInvalidDefinition, i)
		}
		if _, exists := ids[action.ID]; exists {
			return fmt.Errorf("%w: duplicate action id %q", ErrInvalidDefinition, action.ID)
		}
		ids[action.ID] = struct{}{}

		targets := make(map[TargetKind]struct{}, len(action.Targets))
		hasPointTarget := false
		for _, target := range action.Targets {
			if !validTargetKind(target) {
				return fmt.Errorf("%w: action %q target %q", ErrInvalidDefinition, action.ID, target)
			}
			if _, exists := targets[target]; exists {
				return fmt.Errorf("%w: action %q duplicate target %q", ErrInvalidDefinition, action.ID, target)
			}
			targets[target] = struct{}{}
			if target == TargetPoint {
				hasPointTarget = true
			}
		}

		switch effectiveEffect(action.Effect) {
		case EffectDamage:
			if action.BaseDamage == 0 || !validDamageType(action.DamageType) || action.ReviveHPPercent != 0 {
				return fmt.Errorf("%w: damage action %q", ErrInvalidDefinition, action.ID)
			}
			if hasPointTarget && !positiveFinite(action.HitRadius) {
				return fmt.Errorf("%w: point damage action %q requires hit_radius", ErrInvalidDefinition, action.ID)
			}
		case EffectResurrect:
			if action.BaseDamage != 0 || action.DamageType != "" || action.ReviveHPPercent == 0 || action.ReviveHPPercent > 100 || action.HitRadius != 0 {
				return fmt.Errorf("%w: resurrect action %q", ErrInvalidDefinition, action.ID)
			}
			if len(action.Targets) != 1 || action.Targets[0] != TargetEntity {
				return fmt.Errorf("%w: resurrect action %q must target entity only", ErrInvalidDefinition, action.ID)
			}
		default:
			return fmt.Errorf("%w: action %q effect %q", ErrInvalidDefinition, action.ID, action.Effect)
		}
	}
	return nil
}

func NewService(definitions []ActionDefinition) (*Service, error) {
	normalized := make([]ActionDefinition, len(definitions))
	for i, action := range definitions {
		normalized[i] = normalizeAction(action)
	}
	definition := Definition{SchemaVersion: SchemaVersion, Revision: "runtime", Actions: normalized}
	if err := Validate(definition); err != nil {
		return nil, err
	}
	actions := make(map[string]ActionDefinition, len(normalized))
	for _, action := range normalized {
		copy := action
		copy.Targets = append([]TargetKind(nil), action.Targets...)
		actions[action.ID] = copy
	}
	return &Service{actions: actions, nextUseTick: make(map[cooldownKey]uint64)}, nil
}

func (s *Service) PrepareIntent(intent Intent, tick uint64) (PreparedAction, error) {
	return s.Prepare(intent.ActorEntityID, intent.ActionID, intent.Target, tick)
}

func (s *Service) Prepare(actorEntityID world.EntityID, actionID string, target Target, tick uint64) (PreparedAction, error) {
	action, ok := s.actions[actionID]
	if !ok {
		return PreparedAction{}, ErrUnknownAction
	}
	if !containsTarget(action.Targets, target.Kind) {
		return PreparedAction{}, ErrTargetNotAllowed
	}
	switch target.Kind {
	case TargetPoint:
		if !target.HasPoint || !finite(target.PointX) || !finite(target.PointZ) {
			return PreparedAction{}, ErrTargetNotAllowed
		}
	default:
		if target.ID == "" {
			return PreparedAction{}, ErrTargetNotAllowed
		}
	}
	key := cooldownKey{entityID: actorEntityID, actionID: actionID}
	if next := s.nextUseTick[key]; next != 0 && tick < next {
		return PreparedAction{}, ErrActionCooldown
	}
	s.nextInstanceID++
	if s.nextInstanceID == 0 {
		s.nextInstanceID++
	}
	prepared := PreparedAction{
		ActionInstanceID: s.nextInstanceID,
		ActorEntityID:    actorEntityID,
		Definition:       action,
		Target:           target,
	}
	if action.Effect == EffectDamage {
		prepared.Damage = Damage{
			Source: DamageSource{ActorEntityID: actorEntityID, ActionID: actionID},
			Type:   action.DamageType,
			Amount: action.BaseDamage,
		}
	}
	return prepared, nil
}

func (s *Service) Commit(action PreparedAction, tick uint64, delta time.Duration) {
	key := cooldownKey{entityID: action.ActorEntityID, actionID: action.Definition.ID}
	s.nextUseTick[key] = tick + cooldownTicks(action.Definition.CooldownSeconds, delta)
}

func normalizeAction(action ActionDefinition) ActionDefinition {
	action.Effect = effectiveEffect(action.Effect)
	return action
}

func effectiveEffect(effect ActionEffect) ActionEffect {
	if effect == "" {
		return EffectDamage
	}
	return effect
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
	case TargetGate, TargetEntity, TargetPoint:
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
	return value > 0 && finite(value)
}

func finite(value float32) bool {
	f := float64(value)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
