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

const SchemaVersion uint16 = 1

var (
	ErrUnsupportedSchema = errors.New("respawnpolicy: unsupported schema version")
	ErrInvalidDefinition = errors.New("respawnpolicy: invalid definition")
	ErrInvalidTickRate   = errors.New("respawnpolicy: invalid tick rate")
	ErrInvalidEntity     = errors.New("respawnpolicy: invalid entity")
	ErrUnknownSpawnPoint = errors.New("respawnpolicy: unknown spawn point")
	ErrScheduleOverflow  = errors.New("respawnpolicy: due tick overflow")
)

type SpawnPoint struct {
	ID    string        `json:"id"`
	X     float32       `json:"x"`
	Y     float32       `json:"y"`
	Z     float32       `json:"z"`
	Layer world.LayerID `json:"layer"`
}

func (p SpawnPoint) Position() world.Position {
	return world.Position{X: p.X, Y: p.Y, Z: p.Z, Layer: p.Layer}
}

type Definition struct {
	SchemaVersion       uint16       `json:"schema_version"`
	Revision            string       `json:"revision"`
	RespawnDelaySeconds float64      `json:"respawn_delay_seconds"`
	DefaultSpawnPoint   string       `json:"default_spawn_point"`
	SpawnPoints         []SpawnPoint `json:"spawn_points"`
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
	if d.Revision == "" || d.DefaultSpawnPoint == "" || !positiveFinite64(d.RespawnDelaySeconds) {
		return fmt.Errorf("%w: revision/default_spawn_point/respawn_delay_seconds", ErrInvalidDefinition)
	}
	if len(d.SpawnPoints) == 0 {
		return fmt.Errorf("%w: at least one spawn point is required", ErrInvalidDefinition)
	}

	ids := make(map[string]struct{}, len(d.SpawnPoints))
	for i, point := range d.SpawnPoints {
		if point.ID == "" || !finite32(point.X) || !finite32(point.Y) || !finite32(point.Z) {
			return fmt.Errorf("%w: spawn_points[%d]", ErrInvalidDefinition, i)
		}
		if _, exists := ids[point.ID]; exists {
			return fmt.Errorf("%w: duplicate spawn point id %q", ErrInvalidDefinition, point.ID)
		}
		ids[point.ID] = struct{}{}
	}
	if _, ok := ids[d.DefaultSpawnPoint]; !ok {
		return fmt.Errorf("%w: default spawn point %q missing", ErrInvalidDefinition, d.DefaultSpawnPoint)
	}
	return nil
}

// ValidateAgainstWorld 確保 Server-only spawn point 真的落在共享 Gameplay World 的 surface 上，
// 並避免預設出生點位於初始啟用的 movement blocker 內。它不修改 Gameplay World schema / SHA。
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
	SpawnPointID string
	Position     world.Position
	DueTick      uint64
}

// Service 只由 WorldRuntime owner goroutine mutate；外部 subsystem 必須透過 Runtime command queue。
type Service struct {
	revision     string
	delayTicks   uint64
	defaultPoint string
	points       map[string]SpawnPoint
	checkpoints  map[world.EntityID]string
	pending      map[world.EntityID]Scheduled
	dueScratch   []Scheduled
}

func NewService(definition Definition, tickRateHz int) (*Service, error) {
	if err := Validate(definition); err != nil {
		return nil, err
	}
	if tickRateHz <= 0 {
		return nil, ErrInvalidTickRate
	}
	delayFloat := math.Ceil(definition.RespawnDelaySeconds * float64(tickRateHz))
	if math.IsNaN(delayFloat) || math.IsInf(delayFloat, 0) || delayFloat <= 0 || delayFloat > float64(^uint64(0)) {
		return nil, ErrInvalidDefinition
	}
	points := make(map[string]SpawnPoint, len(definition.SpawnPoints))
	for _, point := range definition.SpawnPoints {
		points[point.ID] = point
	}
	return &Service{
		revision:     definition.Revision,
		delayTicks:   uint64(delayFloat),
		defaultPoint: definition.DefaultSpawnPoint,
		points:       points,
		checkpoints:  make(map[world.EntityID]string),
		pending:      make(map[world.EntityID]Scheduled),
	}, nil
}

func (s *Service) Revision() string      { return s.revision }
func (s *Service) DelayTicks() uint64    { return s.delayTicks }
func (s *Service) SpawnPointCount() int  { return len(s.points) }

func (s *Service) SetCheckpoint(entityID world.EntityID, spawnPointID string) error {
	if entityID == 0 {
		return ErrInvalidEntity
	}
	if _, ok := s.points[spawnPointID]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownSpawnPoint, spawnPointID)
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

func (s *Service) selectedPoint(entityID world.EntityID) SpawnPoint {
	pointID := s.defaultPoint
	if checkpoint, ok := s.checkpoints[entityID]; ok {
		pointID = checkpoint
	}
	return s.points[pointID]
}

// Schedule 在 defeat 當下綁定目的地。之後修改 checkpoint 不會改寫這次已排定的 death outcome。
func (s *Service) Schedule(entityID world.EntityID, defeatedTick uint64) (Scheduled, error) {
	if entityID == 0 {
		return Scheduled{}, ErrInvalidEntity
	}
	if s.delayTicks > ^uint64(0)-defeatedTick {
		return Scheduled{}, ErrScheduleOverflow
	}
	point := s.selectedPoint(entityID)
	scheduled := Scheduled{
		EntityID:     entityID,
		SpawnPointID: point.ID,
		Position:     point.Position(),
		DueTick:      defeatedTick + s.delayTicks,
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
// 維持 deterministic owner ordering；成功的 S3-F.2 authoritative transition會呼叫 Cancel確認進度。
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
