// Package deathpenalty 定義 Server-owned death penalty policy 與 defeat revision exactly-once 邊界。
package deathpenalty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

const SchemaVersion uint16 = 1

var (
	ErrUnsupportedSchema = errors.New("deathpenalty: unsupported schema version")
	ErrInvalidDefinition = errors.New("deathpenalty: invalid definition")
	ErrInvalidEntity     = errors.New("deathpenalty: invalid entity")
	ErrInvalidRevision   = errors.New("deathpenalty: invalid defeat revision")
	ErrRevisionRegression = errors.New("deathpenalty: defeat revision regression")
)

type Definition struct {
	SchemaVersion               uint16                       `json:"schema_version"`
	Revision                    string                       `json:"revision"`
	CheckpointForfeitContexts   []respawnpolicy.DeathContext `json:"checkpoint_forfeit_contexts"`
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

func Validate(definition Definition) error {
	if definition.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, definition.SchemaVersion, SchemaVersion)
	}
	if definition.Revision == "" {
		return fmt.Errorf("%w: revision", ErrInvalidDefinition)
	}
	seen := make(map[respawnpolicy.DeathContext]struct{}, len(definition.CheckpointForfeitContexts))
	for i, context := range definition.CheckpointForfeitContexts {
		if !validDeathContext(context) {
			return fmt.Errorf("%w: checkpoint_forfeit_contexts[%d]=%q", ErrInvalidDefinition, i, context)
		}
		if _, duplicate := seen[context]; duplicate {
			return fmt.Errorf("%w: duplicate checkpoint forfeit context %q", ErrInvalidDefinition, context)
		}
		seen[context] = struct{}{}
	}
	return nil
}

func validDeathContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege:
		return true
	default:
		return false
	}
}

type Decision struct {
	ForfeitCheckpoint bool
}

// Service 只由 WorldRuntime owner goroutine mutate。appliedRevision 讓同一 defeat revision
// 在 retry / duplicated internal call 下保持 exactly-once；較舊 revision 則視為 invariant fault。
type Service struct {
	revision          string
	checkpointForfeit map[respawnpolicy.DeathContext]struct{}
	appliedRevision   map[world.EntityID]uint64
}

func NewService(definition Definition) (*Service, error) {
	if err := Validate(definition); err != nil {
		return nil, err
	}
	forfeit := make(map[respawnpolicy.DeathContext]struct{}, len(definition.CheckpointForfeitContexts))
	for _, context := range definition.CheckpointForfeitContexts {
		forfeit[context] = struct{}{}
	}
	return &Service{
		revision:          definition.Revision,
		checkpointForfeit: forfeit,
		appliedRevision:   make(map[world.EntityID]uint64),
	}, nil
}

func (s *Service) Revision() string { return s.revision }

func (s *Service) ForfeitsCheckpoint(context respawnpolicy.DeathContext) bool {
	_, ok := s.checkpointForfeit[context]
	return ok
}

// Apply consumes one authoritative defeat revision. applied=false only代表同 revision 已處理過；
// 即使該 context 沒有 penalty effect，revision仍會被記錄，避免同一 death outcome 被重新判定。
func (s *Service) Apply(entityID world.EntityID, defeatRevision uint64, context respawnpolicy.DeathContext) (decision Decision, applied bool, err error) {
	if entityID == 0 {
		return Decision{}, false, ErrInvalidEntity
	}
	if defeatRevision == 0 {
		return Decision{}, false, ErrInvalidRevision
	}
	if !validDeathContext(context) {
		return Decision{}, false, fmt.Errorf("%w: death context %q", ErrInvalidDefinition, context)
	}
	last := s.appliedRevision[entityID]
	if defeatRevision < last {
		return Decision{}, false, fmt.Errorf("%w: entity=%d got=%d last=%d", ErrRevisionRegression, entityID, defeatRevision, last)
	}
	if defeatRevision == last {
		return Decision{}, false, nil
	}
	s.appliedRevision[entityID] = defeatRevision
	return Decision{ForfeitCheckpoint: s.ForfeitsCheckpoint(context)}, true, nil
}

func (s *Service) AppliedRevision(entityID world.EntityID) (uint64, bool) {
	revision, ok := s.appliedRevision[entityID]
	return revision, ok
}

func (s *Service) Remove(entityID world.EntityID) {
	delete(s.appliedRevision, entityID)
}
