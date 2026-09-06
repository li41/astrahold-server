// Package siegeownership provides durable single-writer storage for castle ownership.
package siegeownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/li41/astrahold-server/internal/siege"
)

const SchemaVersion uint16 = 1

var (
	ErrInvalidRoot          = errors.New("siegeownership: invalid store root")
	ErrInvalidWorldID       = errors.New("siegeownership: invalid world id")
	ErrInvalidInitialOwner  = errors.New("siegeownership: invalid initial owner")
	ErrOwnershipUnavailable = errors.New("siegeownership: ownership record unavailable")
	ErrRevisionConflict     = errors.New("siegeownership: revision conflict")
	ErrRevisionOverflow     = errors.New("siegeownership: revision overflow")
	ErrCorruptRecord        = errors.New("siegeownership: corrupt record")
)

type Store struct {
	mu   sync.Mutex
	root string
}

type wireRecord struct {
	SchemaVersion       uint16 `json:"schema_version"`
	WorldID             string `json:"world_id"`
	Revision            uint64 `json:"revision"`
	OwnerID             string `json:"owner_id"`
	PreviousOwnerID     string `json:"previous_owner_id,omitempty"`
	LastTransferMatchID string `json:"last_transfer_match_id,omitempty"`
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

// Load returns the durable ownership snapshot for one authoritative gameplay world.
// Missing records return (zero, false, nil).
func (s *Store) Load(worldID string) (siege.CastleOwnershipState, bool, error) {
	if worldID == "" {
		return siege.CastleOwnershipState{}, false, ErrInvalidWorldID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(worldID)
}

// LoadOrCreate initializes a world exactly once with the configured defender as owner.
// Existing durable ownership always wins over the current process's default config.
func (s *Store) LoadOrCreate(worldID, initialOwnerID string) (siege.CastleOwnershipState, bool, error) {
	if worldID == "" {
		return siege.CastleOwnershipState{}, false, ErrInvalidWorldID
	}
	if initialOwnerID == "" {
		return siege.CastleOwnershipState{}, false, ErrInvalidInitialOwner
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists, err := s.loadLocked(worldID)
	if err != nil {
		return siege.CastleOwnershipState{}, false, err
	}
	if exists {
		return current, false, nil
	}
	initial := siege.CastleOwnershipState{Revision: 1, OwnerID: initialOwnerID}
	if err := s.writeLocked(worldID, initial); err != nil {
		return siege.CastleOwnershipState{}, false, err
	}
	return initial, true, nil
}

// Commit applies one ownership transfer with optimistic revision fencing. A repeated call for
// the exact already-committed transfer is idempotent, covering a crash/error between durable
// Store commit and in-memory match publication.
func (s *Store) Commit(worldID string, transfer siege.CastleOwnershipTransfer) (siege.CastleOwnershipState, error) {
	if worldID == "" {
		return siege.CastleOwnershipState{}, ErrInvalidWorldID
	}
	if !transfer.Valid() {
		return siege.CastleOwnershipState{}, siege.ErrInvalidCastleTransfer
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists, err := s.loadLocked(worldID)
	if err != nil {
		return siege.CastleOwnershipState{}, err
	}
	if !exists {
		return siege.CastleOwnershipState{}, ErrOwnershipUnavailable
	}
	if current.Revision == transfer.ExpectedRevision {
		if current.OwnerID != transfer.PreviousOwnerID {
			return siege.CastleOwnershipState{}, revisionConflict(worldID, transfer, current)
		}
		if current.OwnerID == transfer.OwnerID {
			return current, nil
		}
		if current.Revision == ^uint64(0) {
			return siege.CastleOwnershipState{}, ErrRevisionOverflow
		}
		next := siege.CastleOwnershipState{
			Revision:            current.Revision + 1,
			OwnerID:             transfer.OwnerID,
			PreviousOwnerID:     current.OwnerID,
			LastTransferMatchID: transfer.MatchID,
		}
		if err := s.writeLocked(worldID, next); err != nil {
			return siege.CastleOwnershipState{}, err
		}
		return next, nil
	}
	if transfer.ExpectedRevision != ^uint64(0) && current.Revision == transfer.ExpectedRevision+1 &&
		current.PreviousOwnerID == transfer.PreviousOwnerID && current.OwnerID == transfer.OwnerID && current.LastTransferMatchID == transfer.MatchID {
		return current, nil
	}
	return siege.CastleOwnershipState{}, revisionConflict(worldID, transfer, current)
}

func (s *Store) loadLocked(worldID string) (siege.CastleOwnershipState, bool, error) {
	data, err := os.ReadFile(s.recordPath(worldID))
	if errors.Is(err, os.ErrNotExist) {
		return siege.CastleOwnershipState{}, false, nil
	}
	if err != nil {
		return siege.CastleOwnershipState{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireRecord
	if err := decoder.Decode(&wire); err != nil {
		return siege.CastleOwnershipState{}, false, fmt.Errorf("%w: decode world=%s: %v", ErrCorruptRecord, worldID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return siege.CastleOwnershipState{}, false, fmt.Errorf("%w: trailing JSON value", ErrCorruptRecord)
		}
		return siege.CastleOwnershipState{}, false, fmt.Errorf("%w: trailing data: %v", ErrCorruptRecord, err)
	}
	if wire.SchemaVersion != SchemaVersion || wire.WorldID != worldID || wire.Revision == 0 || wire.OwnerID == "" ||
		((wire.PreviousOwnerID == "") != (wire.LastTransferMatchID == "")) {
		return siege.CastleOwnershipState{}, false, ErrCorruptRecord
	}
	return siege.CastleOwnershipState{
		Revision:            wire.Revision,
		OwnerID:             wire.OwnerID,
		PreviousOwnerID:     wire.PreviousOwnerID,
		LastTransferMatchID: wire.LastTransferMatchID,
	}, true, nil
}

func (s *Store) writeLocked(worldID string, state siege.CastleOwnershipState) error {
	wire := wireRecord{
		SchemaVersion:       SchemaVersion,
		WorldID:             worldID,
		Revision:            state.Revision,
		OwnerID:             state.OwnerID,
		PreviousOwnerID:     state.PreviousOwnerID,
		LastTransferMatchID: state.LastTransferMatchID,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.root, ".siege-ownership.tmp-")
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
	if err := os.Rename(tmpName, s.recordPath(worldID)); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return syncDirectory(s.root)
}

func (s *Store) recordPath(worldID string) string {
	sum := sha256.Sum256([]byte(worldID))
	return filepath.Join(s.root, fmt.Sprintf("%x.json", sum[:]))
}

func revisionConflict(worldID string, transfer siege.CastleOwnershipTransfer, current siege.CastleOwnershipState) error {
	return fmt.Errorf("%w: world=%s expected_revision=%d current_revision=%d previous_owner=%s current_owner=%s next_owner=%s match=%s", ErrRevisionConflict, worldID, transfer.ExpectedRevision, current.Revision, transfer.PreviousOwnerID, current.OwnerID, transfer.OwnerID, transfer.MatchID)
}
