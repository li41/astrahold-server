// Package respawnpolicy 定義玩家層 respawn 的 Server-owned policy state。
package respawnpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

const SchemaVersion uint16 = 2

var (
	ErrUnsupportedSchema       = errors.New("respawnpolicy: unsupported schema version")
	ErrInvalidDefinition       = errors.New("respawnpolicy: invalid definition")
	ErrInvalidTickRate         = errors.New("respawnpolicy: invalid tick rate")
	ErrInvalidEntity           = errors.New("respawnpolicy: invalid entity")
	ErrUnknownSpawnPoint       = errors.New("respawnpolicy: unknown spawn point")
	ErrUnknownDeathContext     = errors.New("respawnpolicy: unknown death context")
	ErrCheckpointNotAcquirable = errors.New("respawnpolicy: spawn point is not an acquirable checkpoint")
	ErrCheckpointWrongLayer    = errors.New("respawnpolicy: checkpoint wrong layer")
	ErrCheckpointOutOfRange    = errors.New("respawnpolicy: checkpoint out of range")
	ErrScheduleOverflow        = errors.New("respawnpolicy: due tick overflow")
)

type DeathContext string

const (
	DeathContextPvE   DeathContext = "pve"
	DeathContextPvP   DeathContext = "pvp"
	DeathContextSiege DeathContext = "siege"
)

var requiredDeathContexts = []DeathContext{DeathContextPvE, DeathContextPvP, DeathContextSiege}

type SpawnClass string

const (
	SpawnClassSafe       SpawnClass = "safe"
	SpawnClassCheckpoint SpawnClass = "checkpoint"
	SpawnClassSiege      SpawnClass = "siege"
)

func validSpawnClass(class SpawnClass) bool {
	switch class {
	case SpawnClassSafe, SpawnClassCheckpoint, SpawnClassSiege:
		return true
	default:
		return false
	}
}

type SpawnPoint struct {
	ID                         string        `json:"id"`
	Class                      SpawnClass    `json:"class"`
	X                          float32       `json:"x"`
	Y                          float32       `json:"y"`
	Z                          float32       `json:"z"`
	Layer                      world.LayerID `json:"layer"`
	CheckpointActivationRadius float32       `json:"checkpoint_activation_radius"`
}

func (p SpawnPoint) Position() world.Position {
	return world.Position{X: p.X, Y: p.Y, Z: p.Z, Layer: p.Layer}
}

type ContextRule struct {
	Context             DeathContext `json:"context"`
	RespawnDelaySeconds float64      `json:"respawn_delay_seconds"`
	DefaultSpawnPoint   string       `json:"default_spawn_point"`
	AllowedSpawnClasses []SpawnClass `json:"allowed_spawn_classes"`
}

type Definition struct {
	SchemaVersion uint16        `json:"schema_version"`
	Revision      string        `json:"revision"`
	SpawnPoints   []SpawnPoint  `json:"spawn_points"`
	Contexts      []ContextRule `json:"contexts"`
}

type Loaded struct {
	Definition Definition
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

func Validate(d Definition) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, d.SchemaVersion, SchemaVersion)
	}
	if d.Revision == "" {
		return fmt.Errorf("%w: revision", ErrInvalidDefinition)
	}
	if len(d.SpawnPoints) == 0 {
		return fmt.Errorf("%w: at least one spawn point is required", ErrInvalidDefinition)
	}

	points := make(map[string]SpawnPoint, len(d.SpawnPoints))
	classCounts := make(map[SpawnClass]int)
	for i, point := range d.SpawnPoints {
		if point.ID == "" || !validSpawnClass(point.Class) || !finite32(point.X) || !finite32(point.Y) || !finite32(point.Z) || !finite32(point.CheckpointActivationRadius) || point.CheckpointActivationRadius < 0 {
			return fmt.Errorf("%w: spawn_points[%d]", ErrInvalidDefinition, i)
		}
		if point.Class == SpawnClassCheckpoint {
			if point.CheckpointActivationRadius <= 0 {
				return fmt.Errorf("%w: checkpoint spawn point %q requires positive activation radius", ErrInvalidDefinition, point.ID)
			}
		} else if point.CheckpointActivationRadius != 0 {
			return fmt.Errorf("%w: non-checkpoint spawn point %q cannot define activation radius", ErrInvalidDefinition, point.ID)
		}
		if _, exists := points[point.ID]; exists {
			return fmt.Errorf("%w: duplicate spawn point id %q", ErrInvalidDefinition, point.ID)
		}
		points[point.ID] = point
		classCounts[point.Class]++
	}

	if len(d.Contexts) != len(requiredDeathContexts) {
		return fmt.Errorf("%w: exactly pve/pvp/siege context rules are required", ErrInvalidDefinition)
	}
	seenContexts := make(map[DeathContext]struct{}, len(d.Contexts))
	for i, rule := range d.Contexts {
		if !validDeathContext(rule.Context) || !positiveFinite64(rule.RespawnDelaySeconds) || rule.DefaultSpawnPoint == "" || len(rule.AllowedSpawnClasses) == 0 {
			return fmt.Errorf("%w: contexts[%d]", ErrInvalidDefinition, i)
		}
		if _, exists := seenContexts[rule.Context]; exists {
			return fmt.Errorf("%w: duplicate death context %q", ErrInvalidDefinition, rule.Context)
		}
		seenContexts[rule.Context] = struct{}{}

		allowed := make(map[SpawnClass]struct{}, len(rule.AllowedSpawnClasses))
		for _, class := range rule.AllowedSpawnClasses {
			if !validSpawnClass(class) {
				return fmt.Errorf("%w: context %q has unknown spawn class %q", ErrInvalidDefinition, rule.Context, class)
			}
			if _, duplicate := allowed[class]; duplicate {
				return fmt.Errorf("%w: context %q duplicate spawn class %q", ErrInvalidDefinition, rule.Context, class)
			}
			if classCounts[class] == 0 {
				return fmt.Errorf("%w: context %q allows missing spawn class %q", ErrInvalidDefinition, rule.Context, class)
			}
			allowed[class] = struct{}{}
		}
		defaultPoint, ok := points[rule.DefaultSpawnPoint]
		if !ok {
			return fmt.Errorf("%w: context %q default spawn point %q missing", ErrInvalidDefinition, rule.Context, rule.DefaultSpawnPoint)
		}
		if _, ok := allowed[defaultPoint.Class]; !ok {
			return fmt.Errorf("%w: context %q default spawn class %q is not allowed", ErrInvalidDefinition, rule.Context, defaultPoint.Class)
		}
	}
	for _, context := range requiredDeathContexts {
		if _, ok := seenContexts[context]; !ok {
			return fmt.Errorf("%w: missing death context %q", ErrInvalidDefinition, context)
		}
	}
	return nil
}

func validDeathContext(context DeathContext) bool {
	switch context {
	case DeathContextPvE, DeathContextPvP, DeathContextSiege:
		return true
	default:
		return false
	}
}

// ValidateAgainstWorld 確保 Server-only spawn point 真的落在共享 Gameplay World 的 surface 上，
// 並避免出生點位於初始啟用的 movement blocker 內。它不修改 Gameplay World schema / SHA。
func ValidateAgainstWorld(d Definition, gameplay gameplayworld.Definition) error {
	if err := Validate(d); err != nil {
		return err
	}
	if err := gameplayworld.Validate(gameplay); err != nil {
		return err
	}
	for _, point := range d.SpawnPoints {
		position := point.Position()
		var surface *gameplayworld.Surface
		for i := range gameplay.Surfaces {
			candidate := &gameplay.Surfaces[i]
			if candidate.Layer == position.Layer && candidate.Bounds.Contains(position.X, position.Z) {
				surface = candidate
				break
			}
		}
		if surface == nil {
			return fmt.Errorf("%w: spawn point %q is outside layer %d surfaces", ErrInvalidDefinition, point.ID, point.Layer)
		}
		expectedY := surface.Plane.HeightAt(position.X, position.Z)
		if math.Abs(float64(expectedY-position.Y)) > 0.001 {
			return fmt.Errorf("%w: spawn point %q y=%g surface_y=%g", ErrInvalidDefinition, point.ID, position.Y, expectedY)
		}
		for _, blocker := range gameplay.Blockers {
			if !blocker.Enabled || !blocker.BlocksMovement || blocker.Layer != position.Layer {
				continue
			}
			if blocker.Bounds.Contains(position.X, position.Z) && position.Y >= blocker.MinY && position.Y <= blocker.MaxY {
				return fmt.Errorf("%w: spawn point %q intersects blocker %q", ErrInvalidDefinition, point.ID, blocker.ID)
			}
		}
	}
	return nil
}

type Scheduled struct {
	EntityID     world.EntityID
	Context      DeathContext
	SpawnPointID string
	SpawnClass   SpawnClass
	Position     world.Position
	DueTick      uint64
}

type runtimeRule struct {
	delayTicks   uint64
	defaultPoint string
	allowed      map[SpawnClass]struct{}
}

// Service 只由 WorldRuntime owner goroutine mutate；外部 subsystem 必須透過 Runtime command queue。
type Service struct {
	revision    string
	points      map[string]SpawnPoint
	rules       map[DeathContext]runtimeRule
	checkpoints map[world.EntityID]string
	pending     map[world.EntityID]Scheduled
	dueScratch  []Scheduled
}

func NewService(definition Definition, tickRateHz int) (*Service, error) {
	if err := Validate(definition); err != nil {
		return nil, err
	}
	if tickRateHz <= 0 {
		return nil, ErrInvalidTickRate
	}
	points := make(map[string]SpawnPoint, len(definition.SpawnPoints))
	for _, point := range definition.SpawnPoints {
		points[point.ID] = point
	}
	rules := make(map[DeathContext]runtimeRule, len(definition.Contexts))
	for _, definitionRule := range definition.Contexts {
		delayFloat := math.Ceil(definitionRule.RespawnDelaySeconds * float64(tickRateHz))
		if math.IsNaN(delayFloat) || math.IsInf(delayFloat, 0) || delayFloat <= 0 || delayFloat > float64(^uint64(0)) {
			return nil, ErrInvalidDefinition
		}
		allowed := make(map[SpawnClass]struct{}, len(definitionRule.AllowedSpawnClasses))
		for _, class := range definitionRule.AllowedSpawnClasses {
			allowed[class] = struct{}{}
		}
		rules[definitionRule.Context] = runtimeRule{delayTicks: uint64(delayFloat), defaultPoint: definitionRule.DefaultSpawnPoint, allowed: allowed}
	}
	return &Service{
		revision:    definition.Revision,
		points:      points,
		rules:       rules,
		checkpoints: make(map[world.EntityID]string),
		pending:     make(map[world.EntityID]Scheduled),
	}, nil
}

func (s *Service) Revision() string     { return s.revision }
func (s *Service) SpawnPointCount() int { return len(s.points) }

func (s *Service) DelayTicks(context DeathContext) (uint64, bool) {
	rule, ok := s.rules[context]
	return rule.delayTicks, ok
}

func (s *Service) SpawnPoint(id string) (SpawnPoint, bool) {
	point, ok := s.points[id]
	return point, ok
}

// AcquireCheckpoint 是 gameplay checkpoint 的唯一一般取得路徑：必須是 checkpoint class、
// 同 Layer，而且 actor 的 authoritative Position 必須落在 Server-configured activation radius 內。
func (s *Service) AcquireCheckpoint(entityID world.EntityID, position world.Position, spawnPointID string) error {
	if entityID == 0 {
		return ErrInvalidEntity
	}
	point, ok := s.points[spawnPointID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownSpawnPoint, spawnPointID)
	}
	if point.Class != SpawnClassCheckpoint || point.CheckpointActivationRadius <= 0 {
		return fmt.Errorf("%w: %s", ErrCheckpointNotAcquirable, spawnPointID)
	}
	if position.Layer != point.Layer {
		return fmt.Errorf("%w: entity_layer=%d checkpoint_layer=%d", ErrCheckpointWrongLayer, position.Layer, point.Layer)
	}
	radiusSq := point.CheckpointActivationRadius * point.CheckpointActivationRadius
	if position.DistanceSquared(point.Position()) > radiusSq {
		return fmt.Errorf("%w: %s", ErrCheckpointOutOfRange, spawnPointID)
	}
	s.checkpoints[entityID] = spawnPointID
	return nil
}

func (s *Service) ClearCheckpoint(entityID world.EntityID) {
	delete(s.checkpoints, entityID)
}

func (s *Service) Checkpoint(entityID world.EntityID) (string, bool) {
	id, ok := s.checkpoints[entityID]
	return id, ok
}

func (s *Service) selectedPoint(entityID world.EntityID, rule runtimeRule) SpawnPoint {
	if checkpoint, ok := s.checkpoints[entityID]; ok {
		if point, exists := s.points[checkpoint]; exists {
			if _, allowed := rule.allowed[point.Class]; allowed {
				return point
			}
		}
	}
	return s.points[rule.defaultPoint]
}

// Schedule 在 defeat 當下綁定 death context、目的地與 due tick。之後修改 checkpoint 或 context
// 設定都不會改寫這次已排定的 death outcome。
func (s *Service) Schedule(entityID world.EntityID, defeatedTick uint64, context DeathContext) (Scheduled, error) {
	if entityID == 0 {
		return Scheduled{}, ErrInvalidEntity
	}
	rule, ok := s.rules[context]
	if !ok {
		return Scheduled{}, fmt.Errorf("%w: %s", ErrUnknownDeathContext, context)
	}
	if rule.delayTicks > ^uint64(0)-defeatedTick {
		return Scheduled{}, ErrScheduleOverflow
	}
	point := s.selectedPoint(entityID, rule)
	scheduled := Scheduled{
		EntityID:     entityID,
		Context:      context,
		SpawnPointID: point.ID,
		SpawnClass:   point.Class,
		Position:     point.Position(),
		DueTick:      defeatedTick + rule.delayTicks,
	}
	s.pending[entityID] = scheduled
	return scheduled, nil
}

func (s *Service) Pending(entityID world.EntityID) (Scheduled, bool) {
	value, ok := s.pending[entityID]
	return value, ok
}

// Cancel 是 pending respawn 的 progress-confirm primitive。只有 authoritative respawn成功、
// 明確手動取消或 entity離開世界後才應呼叫；Due selection 本身不前進 truth。
func (s *Service) Cancel(entityID world.EntityID) {
	delete(s.pending, entityID)
}

func (s *Service) Remove(entityID world.EntityID) {
	delete(s.pending, entityID)
	delete(s.checkpoints, entityID)
}

// Due 回傳目前已到期的 respawn，但不刪除 pending。排序固定為 DueTick / EntityID，
// 維持 deterministic owner ordering；成功的 authoritative transition會呼叫 Cancel確認進度。
func (s *Service) Due(tick uint64) []Scheduled {
	s.dueScratch = s.dueScratch[:0]
	for _, scheduled := range s.pending {
		if scheduled.DueTick <= tick {
			s.dueScratch = append(s.dueScratch, scheduled)
		}
	}
	sort.Slice(s.dueScratch, func(i, j int) bool {
		if s.dueScratch[i].DueTick == s.dueScratch[j].DueTick {
			return s.dueScratch[i].EntityID < s.dueScratch[j].EntityID
		}
		return s.dueScratch[i].DueTick < s.dueScratch[j].DueTick
	})
	return s.dueScratch
}

func finite32(v float32) bool {
	f := float64(v)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func positiveFinite64(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}
