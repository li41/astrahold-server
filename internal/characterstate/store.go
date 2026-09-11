// Package characterstate provides durable, optimistic-concurrency storage for
// trusted character core state. It intentionally does not perform runtime restore;
// that integration belongs to worldruntime.
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
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	LegacySchemaVersion        uint16 = 1
	RespawnSchemaVersion       uint16 = 2
	ResourceSchemaVersion      uint16 = 3
	InventorySchemaVersion     uint16 = 4
	EquipmentSchemaVersion     uint16 = 5
	ClassSchemaVersion         uint16 = 6
	LoadoutSchemaVersion       uint16 = 7
	LearnedSkillsSchemaVersion uint16 = 8
	ClasslessSchemaVersion     uint16 = 9
	SchemaVersion              uint16 = ClasslessSchemaVersion
	LegacyDefaultMaxMP         uint32 = 100
)

var (
	ErrInvalidRoot        = errors.New("characterstate: invalid store root")
	ErrIdentityNotDurable = errors.New("characterstate: identity is not trusted durable identity")
	ErrInvalidSnapshot    = errors.New("characterstate: invalid snapshot")
	ErrRevisionConflict   = errors.New("characterstate: revision conflict")
	ErrRevisionOverflow   = errors.New("characterstate: revision overflow")
	ErrCorruptRecord      = errors.New("characterstate: corrupt record")
)

type WorldRef struct {
	WorldID        string
	Revision       string
	GameplaySHA256 string
}

type DefeatedRespawn struct {
	Context        respawnpolicy.DeathContext
	SpawnPointID   string
	SpawnClass     respawnpolicy.SpawnClass
	Position       world.Position
	RemainingTicks uint64
	CheckpointID   string
}

type Snapshot struct {
	World WorldRef
	// ClassID is retained only as an in-process legacy v27 compatibility carrier while
	// the old initial-class transaction is retired. Schema v9 never writes or restores it.
	ClassID       classid.ID
	HP            uint32
	MaxHP         uint32
	MP            uint32
	MaxMP         uint32
	Defeated      bool
	Position      world.Position
	Yaw           float32
	Respawn       DefeatedRespawn
	Inventory     InventoryState
	CombatLoadout skillloadout.Slots
	LearnedSkills learnedskills.Set
}

type Record struct {
	SchemaVersion uint16
	CharacterID   characteridentity.ID
	Revision      uint64
	Snapshot      Snapshot
}

type Store struct {
	mu   sync.Mutex
	root string
}

type wireDefeatedRespawn struct {
	Context        respawnpolicy.DeathContext `json:"context"`
	SpawnPointID   string                     `json:"spawn_point_id"`
	SpawnClass     respawnpolicy.SpawnClass   `json:"spawn_class"`
	X              float32                    `json:"x"`
	Y              float32                    `json:"y"`
	Z              float32                    `json:"z"`
	Layer          world.LayerID              `json:"layer"`
	RemainingTicks uint64                     `json:"remaining_ticks"`
	CheckpointID   string                     `json:"checkpoint_id,omitempty"`
}

type wireRecord struct {
	SchemaVersion   uint16                `json:"schema_version"`
	CharacterID     string                `json:"character_id"`
	Revision        uint64                `json:"revision"`
	WorldID         string                `json:"world_id"`
	WorldRevision   string                `json:"world_revision"`
	GameplaySHA256  string                `json:"gameplay_sha256"`
	ClassID         string                `json:"class_id,omitempty"`
	HP              uint32                `json:"hp"`
	MaxHP           uint32                `json:"max_hp"`
	MP              uint32                `json:"mp,omitempty"`
	MaxMP           uint32                `json:"max_mp,omitempty"`
	Defeated        bool                  `json:"defeated"`
	X               float32               `json:"x"`
	Y               float32               `json:"y"`
	Z               float32               `json:"z"`
	Layer           world.LayerID         `json:"layer"`
	Yaw             float32               `json:"yaw"`
	DefeatedRespawn *wireDefeatedRespawn `json:"defeated_respawn,omitempty"`
	Inventory        InventoryState        `json:"inventory,omitempty"`
	CombatLoadout    []string              `json:"combat_loadout,omitempty"`
	LearnedSkills    []string              `json:"learned_skills,omitempty"`
}

func Open(root string) (*Store, error) {
	if root == "" { return nil, ErrInvalidRoot }
	clean := filepath.Clean(root)
	if err := os.MkdirAll(clean, 0o750); err != nil { return nil, err }
	return &Store{root: clean}, nil
}

func (s *Store) Path() string { return s.root }

// Load accepts v1-v9 records. v1/v2 predate MP and migrate to the legacy full resource pool.
// v1-v3 predate inventory persistence. v4 persists MainHand only; v5 adds durable OffHand.
// v6-v8 may contain the retired durable ClassID; it is validated during migration and discarded.
// v7 adds the classless six-slot combat loadout. v8 adds learned skills. v9 retires durable ClassID.
// When loading v7, only configured combat skills are inferred as learned; no other skills are granted.
func (s *Store) Load(identity characteridentity.Binding) (Record, bool, error) {
	if err := validateTrustedIdentity(identity); err != nil { return Record{}, false, err }
	s.mu.Lock(); defer s.mu.Unlock()
	return s.loadLocked(identity)
}

func (s *Store) Save(identity characteridentity.Binding, expectedRevision uint64, snapshot Snapshot) (Record, error) {
	if err := validateTrustedIdentity(identity); err != nil { return Record{}, err }
	inventoryState, err := CanonicalInventoryState(snapshot.Inventory)
	if err != nil { return Record{}, err }
	snapshot.Inventory = inventoryState
	if err := validateSnapshotV9(snapshot); err != nil { return Record{}, err }
	s.mu.Lock(); defer s.mu.Unlock()
	current, exists, err := s.loadLocked(identity)
	if err != nil { return Record{}, err }
	currentRevision := uint64(0)
	if exists { currentRevision = current.Revision }
	if currentRevision != expectedRevision {
		return Record{}, fmt.Errorf("%w: character=%s expected=%d current=%d", ErrRevisionConflict, identity.ID, expectedRevision, currentRevision)
	}
	if expectedRevision == ^uint64(0) { return Record{}, ErrRevisionOverflow }
	// ClassID is intentionally cleared from the durable Record contract at v9. A legacy v27
	// caller may still carry it transiently while completing an in-process compatibility action.
	snapshot.ClassID = ""
	record := Record{SchemaVersion: SchemaVersion, CharacterID: identity.ID, Revision: expectedRevision + 1, Snapshot: snapshot}
	if err := s.writeLocked(record); err != nil { return Record{}, err }
	return record, nil
}

func (s *Store) loadLocked(identity characteridentity.Binding) (Record, bool, error) {
	path := s.recordPath(identity.ID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) { return Record{}, false, nil }
	if err != nil { return Record{}, false, err }
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireRecord
	if err := decoder.Decode(&wire); err != nil { return Record{}, false, fmt.Errorf("%w: decode %s: %v", ErrCorruptRecord, identity.ID, err) }
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil { return Record{}, false, fmt.Errorf("%w: trailing JSON value", ErrCorruptRecord) }
		return Record{}, false, fmt.Errorf("%w: trailing data: %v", ErrCorruptRecord, err)
	}
	if wire.SchemaVersion < LegacySchemaVersion || wire.SchemaVersion > SchemaVersion || wire.CharacterID != string(identity.ID) || wire.Revision == 0 {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion == LegacySchemaVersion && wire.DefeatedRespawn != nil { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion < InventorySchemaVersion && wire.Inventory != (InventoryState{}) { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion == InventorySchemaVersion && wire.Inventory.OffHand != "" { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion < ClassSchemaVersion && wire.ClassID != "" { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion >= ClasslessSchemaVersion && wire.ClassID != "" { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion < LoadoutSchemaVersion && len(wire.CombatLoadout) != 0 { return Record{}, false, ErrCorruptRecord }
	if wire.SchemaVersion < LearnedSkillsSchemaVersion && len(wire.LearnedSkills) != 0 { return Record{}, false, ErrCorruptRecord }

	// Historical ClassID is migration-only input. Validate it so corrupt legacy state does not
	// become silently acceptable, then discard it instead of restoring profession truth.
	if wire.SchemaVersion >= ClassSchemaVersion && wire.SchemaVersion < ClasslessSchemaVersion && wire.ClassID != "" {
		if _, ok := classid.Parse(wire.ClassID); !ok { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot) }
	}
	mp, maxMP := wire.MP, wire.MaxMP
	if wire.SchemaVersion < ResourceSchemaVersion {
		mp, maxMP = LegacyDefaultMaxMP, LegacyDefaultMaxMP
	}
	inventoryState := InventoryState{}
	if wire.SchemaVersion >= InventorySchemaVersion {
		var err error
		inventoryState, err = CanonicalInventoryState(wire.Inventory)
		if err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	combatLoadout := skillloadout.Slots{}
	if wire.SchemaVersion >= LoadoutSchemaVersion {
		var err error
		combatLoadout, err = combatLoadoutFromWire(wire.CombatLoadout)
		if err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	learnedSkills := learnedskills.Set{}
	if wire.SchemaVersion >= LearnedSkillsSchemaVersion {
		var err error
		learnedSkills, err = learnedSkillsFromWire(wire.LearnedSkills)
		if err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	} else if wire.SchemaVersion >= LoadoutSchemaVersion {
		var err error
		learnedSkills, err = learnedSkillsFromCombatLoadout(combatLoadout)
		if err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	record := Record{
		SchemaVersion: wire.SchemaVersion,
		CharacterID: characteridentity.ID(wire.CharacterID),
		Revision: wire.Revision,
		Snapshot: Snapshot{
			World: WorldRef{WorldID: wire.WorldID, Revision: wire.WorldRevision, GameplaySHA256: wire.GameplaySHA256},
			HP: wire.HP, MaxHP: wire.MaxHP, MP: mp, MaxMP: maxMP, Defeated: wire.Defeated,
			Position: world.Position{X: wire.X, Y: wire.Y, Z: wire.Z, Layer: wire.Layer}, Yaw: wire.Yaw,
			Inventory: inventoryState,
			CombatLoadout: combatLoadout,
			LearnedSkills: learnedSkills,
		},
	}
	if wire.DefeatedRespawn != nil {
		record.Snapshot.Respawn = DefeatedRespawn{
			Context: wire.DefeatedRespawn.Context, SpawnPointID: wire.DefeatedRespawn.SpawnPointID,
			SpawnClass: wire.DefeatedRespawn.SpawnClass,
			Position: world.Position{X: wire.DefeatedRespawn.X, Y: wire.DefeatedRespawn.Y, Z: wire.DefeatedRespawn.Z, Layer: wire.DefeatedRespawn.Layer},
			RemainingTicks: wire.DefeatedRespawn.RemainingTicks, CheckpointID: wire.DefeatedRespawn.CheckpointID,
		}
	}
	if err := validateSnapshotBase(record.Snapshot); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	if wire.SchemaVersion >= RespawnSchemaVersion {
		if err := validateRespawnSnapshot(record.Snapshot); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	if wire.SchemaVersion >= ResourceSchemaVersion {
		if record.Snapshot.MaxMP == 0 || record.Snapshot.MP > record.Snapshot.MaxMP { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot) }
	}
	if wire.SchemaVersion >= InventorySchemaVersion {
		if err := validateInventoryState(record.Snapshot.Inventory); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	if wire.SchemaVersion >= LoadoutSchemaVersion {
		if err := validateCombatLoadout(record.Snapshot.CombatLoadout); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	if wire.SchemaVersion >= LearnedSkillsSchemaVersion {
		if err := validateLearnedSkills(record.Snapshot.LearnedSkills); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
		if err := validateCombatLoadoutLearned(record.Snapshot.CombatLoadout, record.Snapshot.LearnedSkills); err != nil { return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err) }
	}
	return record, true, nil
}

func (s *Store) writeLocked(record Record) error {
	inventoryState, err := CanonicalInventoryState(record.Snapshot.Inventory)
	if err != nil { return err }
	wire := wireRecord{
		SchemaVersion: SchemaVersion, CharacterID: string(record.CharacterID), Revision: record.Revision,
		WorldID: record.Snapshot.World.WorldID, WorldRevision: record.Snapshot.World.Revision, GameplaySHA256: record.Snapshot.World.GameplaySHA256,
		HP: record.Snapshot.HP, MaxHP: record.Snapshot.MaxHP, MP: record.Snapshot.MP, MaxMP: record.Snapshot.MaxMP, Defeated: record.Snapshot.Defeated,
		X: record.Snapshot.Position.X, Y: record.Snapshot.Position.Y, Z: record.Snapshot.Position.Z, Layer: record.Snapshot.Position.Layer, Yaw: record.Snapshot.Yaw,
		Inventory: inventoryState,
		CombatLoadout: combatLoadoutToWire(record.Snapshot.CombatLoadout),
		LearnedSkills: learnedSkillsToWire(record.Snapshot.LearnedSkills),
	}
	if record.Snapshot.Defeated {
		respawn := record.Snapshot.Respawn
		wire.DefeatedRespawn = &wireDefeatedRespawn{
			Context: respawn.Context, SpawnPointID: respawn.SpawnPointID, SpawnClass: respawn.SpawnClass,
			X: respawn.Position.X, Y: respawn.Position.Y, Z: respawn.Position.Z, Layer: respawn.Position.Layer,
			RemainingTicks: respawn.RemainingTicks, CheckpointID: respawn.CheckpointID,
		}
	}
	data, err := json.Marshal(wire)
	if err != nil { return err }
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.root, ".character-state.tmp-")
	if err != nil { return err }
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil { cleanup(); return err }
	if err := tmp.Sync(); err != nil { cleanup(); return err }
	if err := tmp.Close(); err != nil { _ = os.Remove(tmpName); return err }
	if err := os.Rename(tmpName, s.recordPath(record.CharacterID)); err != nil { _ = os.Remove(tmpName); return err }
	return syncDirectory(s.root)
}

func (s *Store) recordPath(id characteridentity.ID) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(s.root, fmt.Sprintf("%x.json", sum[:]))
}

func validateTrustedIdentity(identity characteridentity.Binding) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted { return ErrIdentityNotDurable }
	return nil
}

func validateSnapshotV9(snapshot Snapshot) error {
	// ClassID may still be carried by the in-process v27 compatibility transaction, but it is
	// never serialized by schema v9. Validate the transient value while that bridge exists.
	if snapshot.ClassID != "" && !classid.IsCanonical(snapshot.ClassID) { return ErrInvalidSnapshot }
	return validateSnapshotV8(snapshot)
}

func validateSnapshotV8(snapshot Snapshot) error {
	if err := validateSnapshotV7(snapshot); err != nil { return err }
	if err := validateLearnedSkills(snapshot.LearnedSkills); err != nil { return err }
	return validateCombatLoadoutLearned(snapshot.CombatLoadout, snapshot.LearnedSkills)
}

func validateSnapshotV7(snapshot Snapshot) error {
	if err := validateSnapshotV6(snapshot); err != nil { return err }
	return validateCombatLoadout(snapshot.CombatLoadout)
}

func validateSnapshotV6(snapshot Snapshot) error {
	if err := validateSnapshotV5(snapshot); err != nil { return err }
	if snapshot.ClassID != "" && !classid.IsCanonical(snapshot.ClassID) { return ErrInvalidSnapshot }
	return nil
}

func validateSnapshotV5(snapshot Snapshot) error {
	if err := validateSnapshotV3(snapshot); err != nil { return err }
	return validateInventoryState(snapshot.Inventory)
}

func validateSnapshotV4(snapshot Snapshot) error {
	if err := validateSnapshotV3(snapshot); err != nil { return err }
	if snapshot.Inventory.OffHand != "" { return ErrInvalidSnapshot }
	return validateInventoryState(snapshot.Inventory)
}

func validateSnapshotV3(snapshot Snapshot) error {
	if err := validateSnapshotBase(snapshot); err != nil { return err }
	if err := validateRespawnSnapshot(snapshot); err != nil { return err }
	if snapshot.MaxMP == 0 || snapshot.MP > snapshot.MaxMP { return ErrInvalidSnapshot }
	return nil
}

func validateRespawnSnapshot(snapshot Snapshot) error {
	if snapshot.Defeated {
		if !validDeathContext(snapshot.Respawn.Context) || snapshot.Respawn.SpawnPointID == "" || !validSpawnClass(snapshot.Respawn.SpawnClass) || !finitePosition(snapshot.Respawn.Position) {
			return ErrInvalidSnapshot
		}
		return nil
	}
	if snapshot.Respawn != (DefeatedRespawn{}) { return ErrInvalidSnapshot }
	return nil
}

func validateSnapshotBase(snapshot Snapshot) error {
	if snapshot.World.WorldID == "" || snapshot.World.Revision == "" || !validSHA256(snapshot.World.GameplaySHA256) { return ErrInvalidSnapshot }
	if snapshot.MaxHP == 0 || snapshot.HP > snapshot.MaxHP { return ErrInvalidSnapshot }
	if snapshot.Defeated {
		if snapshot.HP != 0 { return ErrInvalidSnapshot }
	} else if snapshot.HP == 0 { return ErrInvalidSnapshot }
	if !finitePosition(snapshot.Position) || !finite32(snapshot.Yaw) { return ErrInvalidSnapshot }
	return nil
}

func validDeathContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege: return true
	default: return false
	}
}
func validSpawnClass(class respawnpolicy.SpawnClass) bool {
	switch class {
	case respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint, respawnpolicy.SpawnClassSiege: return true
	default: return false
	}
}
func finitePosition(position world.Position) bool { return finite32(position.X) && finite32(position.Y) && finite32(position.Z) }
func finite32(value float32) bool { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }
func validSHA256(value string) bool {
	if len(value) != 64 { return false }
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') { continue }
		return false
	}
	return true
}
