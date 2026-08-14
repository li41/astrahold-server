// Package characterstate provides durable, optimistic-concurrency storage for
// trusted character core state. It intentionally does not perform runtime restore;
// that integration belongs to a later bounded stage.
package characterstate

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/world"
)

const SchemaVersion uint16 = 1

var (
	ErrInvalidRoot         = errors.New("characterstate: invalid store root")
	ErrIdentityNotDurable  = errors.New("characterstate: identity is not trusted durable identity")
	ErrInvalidSnapshot     = errors.New("characterstate: invalid snapshot")
	ErrRevisionConflict    = errors.New("characterstate: revision conflict")
	ErrRevisionOverflow    = errors.New("characterstate: revision overflow")
	ErrCorruptRecord       = errors.New("characterstate: corrupt record")
)

type WorldRef struct {
	WorldID        string
	Revision       string
	GameplaySHA256 string
}

type Snapshot struct {
	World    WorldRef
	HP       uint32
	MaxHP    uint32
	Defeated bool
	Position world.Position
	Yaw      float32
}

type Record struct {
	CharacterID characteridentity.ID
	Revision    uint64
	Snapshot    Snapshot
}

type Store struct {
	mu   sync.Mutex
	root string
}

type wireRecord struct {
	SchemaVersion  uint16        `json:"schema_version"`
	CharacterID    string        `json:"character_id"`
	Revision       uint64        `json:"revision"`
	WorldID        string        `json:"world_id"`
	WorldRevision  string        `json:"world_revision"`
	GameplaySHA256 string        `json:"gameplay_sha256"`
	HP             uint32        `json:"hp"`
	MaxHP          uint32        `json:"max_hp"`
	Defeated       bool          `json:"defeated"`
	X              float32       `json:"x"`
	Y              float32       `json:"y"`
	Z              float32       `json:"z"`
	Layer          world.LayerID `json:"layer"`
	Yaw            float32       `json:"yaw"`
}

func Open(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalidRoot
	}
	clean := filepath.Clean(root)
	if err := os.MkdirAll(clean, 0o750); err != nil {
		return nil, err
	}
	return &Store{root: clean}, nil
}

func (s *Store) Path() string { return s.root }

// Load reads one trusted character record. Missing records return (Record{}, false, nil).
// Ephemeral identities are rejected so disposable development sessions cannot silently
// become durable ownership keys.
func (s *Store) Load(identity characteridentity.Binding) (Record, bool, error) {
	if err := validateTrustedIdentity(identity); err != nil {
		return Record{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(identity)
}

// Save is compare-and-swap persistence. expectedRevision=0 means create-only.
// A successful write advances the revision exactly once and is made durable using
// temp-file fsync -> atomic rename -> directory fsync.
func (s *Store) Save(identity characteridentity.Binding, expectedRevision uint64, snapshot Snapshot) (Record, error) {
	if err := validateTrustedIdentity(identity); err != nil {
		return Record{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists, err := s.loadLocked(identity)
	if err != nil {
		return Record{}, err
	}
	currentRevision := uint64(0)
	if exists {
		currentRevision = current.Revision
	}
	if currentRevision != expectedRevision {
		return Record{}, fmt.Errorf("%w: character=%s expected=%d current=%d", ErrRevisionConflict, identity.ID, expectedRevision, currentRevision)
	}
	if expectedRevision == ^uint64(0) {
		return Record{}, ErrRevisionOverflow
	}

	record := Record{CharacterID: identity.ID, Revision: expectedRevision + 1, Snapshot: snapshot}
	if err := s.writeLocked(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) loadLocked(identity characteridentity.Binding) (Record, bool, error) {
	path := s.recordPath(identity.ID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireRecord
	if err := decoder.Decode(&wire); err != nil {
		return Record{}, false, fmt.Errorf("%w: decode %s: %v", ErrCorruptRecord, identity.ID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Record{}, false, fmt.Errorf("%w: trailing JSON value", ErrCorruptRecord)
		}
		return Record{}, false, fmt.Errorf("%w: trailing data: %v", ErrCorruptRecord, err)
	}
	if wire.SchemaVersion != SchemaVersion || wire.CharacterID != string(identity.ID) || wire.Revision == 0 {
		return Record{}, false, ErrCorruptRecord
	}

	record := Record{
		CharacterID: characteridentity.ID(wire.CharacterID),
		Revision:    wire.Revision,
		Snapshot: Snapshot{
			World: WorldRef{WorldID: wire.WorldID, Revision: wire.WorldRevision, GameplaySHA256: wire.GameplaySHA256},
			HP:       wire.HP,
			MaxHP:    wire.MaxHP,
			Defeated: wire.Defeated,
			Position: world.Position{X: wire.X, Y: wire.Y, Z: wire.Z, Layer: wire.Layer},
			Yaw:      wire.Yaw,
		},
	}
	if err := validateSnapshot(record.Snapshot); err != nil {
		return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
	}
	return record, true, nil
}

func (s *Store) writeLocked(record Record) error {
	wire := wireRecord{
		SchemaVersion:  SchemaVersion,
		CharacterID:    string(record.CharacterID),
		Revision:       record.Revision,
		WorldID:        record.Snapshot.World.WorldID,
		WorldRevision:  record.Snapshot.World.Revision,
		GameplaySHA256: record.Snapshot.World.GameplaySHA256,
		HP:             record.Snapshot.HP,
		MaxHP:          record.Snapshot.MaxHP,
		Defeated:       record.Snapshot.Defeated,
		X:              record.Snapshot.Position.X,
		Y:              record.Snapshot.Position.Y,
		Z:              record.Snapshot.Position.Z,
		Layer:          record.Snapshot.Position.Layer,
		Yaw:            record.Snapshot.Yaw,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.root, ".character-state.tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.recordPath(record.CharacterID)); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func (s *Store) recordPath(id characteridentity.ID) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(s.root, fmt.Sprintf("%x.json", sum[:]))
}

func validateTrustedIdentity(identity characteridentity.Binding) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrIdentityNotDurable
	}
	return nil
}

func validateSnapshot(snapshot Snapshot) error {
	if snapshot.World.WorldID == "" || snapshot.World.Revision == "" || !validSHA256(snapshot.World.GameplaySHA256) {
		return ErrInvalidSnapshot
	}
	if snapshot.MaxHP == 0 || snapshot.HP > snapshot.MaxHP {
		return ErrInvalidSnapshot
	}
	if snapshot.Defeated {
		if snapshot.HP != 0 {
			return ErrInvalidSnapshot
		}
	} else if snapshot.HP == 0 {
		return ErrInvalidSnapshot
	}
	for _, value := range []float32{snapshot.Position.X, snapshot.Position.Y, snapshot.Position.Z, snapshot.Yaw} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return ErrInvalidSnapshot
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
