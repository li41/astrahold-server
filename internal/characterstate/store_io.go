// Package characterstate provides durable, optimistic-concurrency storage for
// trusted character core state. It intentionally does not perform runtime restore;
// that integration belongs to worldruntime.
package characterstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

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

// Load accepts v1-v15 records. v1/v2 predate MP and migrate to the legacy full resource pool.
// v1-v3 predate inventory persistence. v4 persists MainHand only; v5 adds durable OffHand.
// v6-v8 may contain the retired durable ClassID; it is validated during migration and discarded.
// v7 adds the six-slot combat loadout. v8 adds learned skills. v9 retires durable ClassID.
// v10 adds unique equipment instances. v11 carries only Strength/Agility and is migrated into the
// formal six-attribute baseline; v12 persists all six formal base attributes. v13 persists selected
// SkinID; v1-v12 migrate to explicit no-skin. v14 persists MapID; v1-v13 migrate missing MapID to
// map1. v15 persists CharacterID-bound stack warehouse; v1-v14 migrate to an initialized empty one.
func (s *Store) Load(identity characteridentity.Binding) (Record, bool, error) {
	if err := validateTrustedIdentity(identity); err != nil {
		return Record{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(identity)
}

func (s *Store) Save(identity characteridentity.Binding, expectedRevision uint64, snapshot Snapshot) (Record, error) {
	if err := validateTrustedIdentity(identity); err != nil {
		return Record{}, err
	}
	snapshot = defaultSnapshotMap(snapshot)
	inventoryState, err := CanonicalInventoryState(snapshot.Inventory)
	if err != nil {
		return Record{}, err
	}
	snapshot.Inventory = inventoryState
	if !snapshot.Warehouse.Initialized && len(snapshot.Warehouse.Items) == 0 {
		snapshot.Warehouse = EmptyWarehouseState()
	}
	warehouseState, err := CanonicalWarehouseState(snapshot.Warehouse)
	if err != nil {
		return Record{}, err
	}
	snapshot.Warehouse = warehouseState
	if err := validateSnapshotV15(snapshot); err != nil {
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
	record := Record{SchemaVersion: SchemaVersion, CharacterID: identity.ID, Revision: expectedRevision + 1, Snapshot: snapshot}
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
	if wire.SchemaVersion < LegacySchemaVersion || wire.SchemaVersion > SchemaVersion || wire.CharacterID != string(identity.ID) || wire.Revision == 0 {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion == LegacySchemaVersion && wire.DefeatedRespawn != nil {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < InventorySchemaVersion && wire.Inventory != (InventoryState{}) {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion == InventorySchemaVersion && wire.Inventory.OffHand != "" {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < ItemInstanceSchemaVersion && wire.Inventory.HasItemInstances() {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < WarehouseSchemaVersion && (wire.Warehouse.Initialized || len(wire.Warehouse.Items) != 0) {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= WarehouseSchemaVersion && !wire.Warehouse.Initialized {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < ClassSchemaVersion && wire.ClassID != "" {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= ClasslessSchemaVersion && wire.ClassID != "" {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < AppearanceSchemaVersion && wire.SkinID != "" {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < MapSchemaVersion && wire.MapID != "" {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= MapSchemaVersion && !validMapID(wire.MapID) {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < LoadoutSchemaVersion && len(wire.CombatLoadout) != 0 {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < LearnedSkillsSchemaVersion && len(wire.LearnedSkills) != 0 {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion < PrimaryStatsSchemaVersion && wire.PrimaryStats != nil {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= PrimaryStatsSchemaVersion && wire.PrimaryStats == nil {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion == PrimaryStatsSchemaVersion && hasSixPrimaryWireFields(wire.PrimaryStats) {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= SixPrimaryStatsSchemaVersion && !hasSixPrimaryWireFields(wire.PrimaryStats) {
		return Record{}, false, ErrCorruptRecord
	}
	if wire.SchemaVersion >= ClassSchemaVersion && wire.SchemaVersion < ClasslessSchemaVersion && wire.ClassID != "" {
		if _, ok := classid.Parse(wire.ClassID); !ok {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
		}
	}
	mp, maxMP := wire.MP, wire.MaxMP
	if wire.SchemaVersion < ResourceSchemaVersion {
		mp, maxMP = LegacyDefaultMaxMP, LegacyDefaultMaxMP
	}
	inventoryState := InventoryState{}
	if wire.SchemaVersion >= InventorySchemaVersion {
		var err error
		inventoryState, err = CanonicalInventoryState(wire.Inventory)
		if err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	warehouseState := EmptyWarehouseState()
	if wire.SchemaVersion >= WarehouseSchemaVersion {
		var err error
		warehouseState, err = CanonicalWarehouseState(wire.Warehouse)
		if err != nil || !warehouseState.Initialized {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
		}
	}
	combatLoadout := skillloadout.Slots{}
	if wire.SchemaVersion >= LoadoutSchemaVersion {
		var err error
		combatLoadout, err = combatLoadoutFromWire(wire.CombatLoadout)
		if err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	learnedSkills := learnedskills.Set{}
	if wire.SchemaVersion >= LearnedSkillsSchemaVersion {
		var err error
		learnedSkills, err = learnedSkillsFromWire(wire.LearnedSkills)
		if err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	} else if wire.SchemaVersion >= LoadoutSchemaVersion {
		var err error
		learnedSkills, err = learnedSkillsFromCombatLoadout(combatLoadout)
		if err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	primaryStats, err := primaryStatsFromWire(wire.SchemaVersion, wire.PrimaryStats)
	if err != nil {
		return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
	}
	mapID := LegacyDefaultMapID
	if wire.SchemaVersion >= MapSchemaVersion {
		mapID = wire.MapID
	}
	skinID := appearance.None
	if wire.SchemaVersion >= AppearanceSchemaVersion {
		skinID = appearance.SkinID(wire.SkinID)
		if !appearance.ValidSelection(skinID) {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
		}
	}
	record := Record{
		SchemaVersion: wire.SchemaVersion,
		CharacterID:   characteridentity.ID(wire.CharacterID),
		Revision:      wire.Revision,
		Snapshot: Snapshot{
			World: WorldRef{MapID: mapID, WorldID: wire.WorldID, Revision: wire.WorldRevision, GameplaySHA256: wire.GameplaySHA256},
			HP: wire.HP, MaxHP: wire.MaxHP, MP: mp, MaxMP: maxMP, Defeated: wire.Defeated,
			Position: world.Position{X: wire.X, Y: wire.Y, Z: wire.Z, Layer: wire.Layer}, Yaw: wire.Yaw,
			Inventory: inventoryState, Warehouse: warehouseState, CombatLoadout: combatLoadout, LearnedSkills: learnedSkills, PrimaryStats: primaryStats, SkinID: skinID,
		},
	}
	if wire.DefeatedRespawn != nil {
		record.Snapshot.Respawn = DefeatedRespawn{
			Context: wire.DefeatedRespawn.Context, SpawnPointID: wire.DefeatedRespawn.SpawnPointID, SpawnClass: wire.DefeatedRespawn.SpawnClass,
			Position: world.Position{X: wire.DefeatedRespawn.X, Y: wire.DefeatedRespawn.Y, Z: wire.DefeatedRespawn.Z, Layer: wire.DefeatedRespawn.Layer},
			RemainingTicks: wire.DefeatedRespawn.RemainingTicks, CheckpointID: wire.DefeatedRespawn.CheckpointID,
		}
	}
	if err := validateSnapshotBase(record.Snapshot); err != nil {
		return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
	}
	if wire.SchemaVersion >= RespawnSchemaVersion {
		if err := validateRespawnSnapshot(record.Snapshot); err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	if wire.SchemaVersion >= ResourceSchemaVersion {
		if record.Snapshot.MaxMP == 0 || record.Snapshot.MP > record.Snapshot.MaxMP {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
		}
	}
	if wire.SchemaVersion >= InventorySchemaVersion {
		if err := validateInventoryState(record.Snapshot.Inventory); err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	if wire.SchemaVersion >= WarehouseSchemaVersion {
		if canonical, err := CanonicalWarehouseState(record.Snapshot.Warehouse); err != nil || !canonical.Initialized {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
		}
	}
	if wire.SchemaVersion >= LoadoutSchemaVersion {
		if err := validateCombatLoadout(record.Snapshot.CombatLoadout); err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	if wire.SchemaVersion >= LearnedSkillsSchemaVersion {
		if err := validateLearnedSkills(record.Snapshot.LearnedSkills); err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
		if err := validateCombatLoadoutLearned(record.Snapshot.CombatLoadout, record.Snapshot.LearnedSkills); err != nil {
			return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, err)
		}
	}
	if wire.SchemaVersion >= AppearanceSchemaVersion && !appearance.ValidSelection(record.Snapshot.SkinID) {
		return Record{}, false, fmt.Errorf("%w: %v", ErrCorruptRecord, ErrInvalidSnapshot)
	}
	return record, true, nil
}

func (s *Store) writeLocked(record Record) error {
	inventoryState, err := CanonicalInventoryState(record.Snapshot.Inventory)
	if err != nil {
		return err
	}
	warehouseState := record.Snapshot.Warehouse
	if !warehouseState.Initialized && len(warehouseState.Items) == 0 {
		warehouseState = EmptyWarehouseState()
	}
	warehouseState, err = CanonicalWarehouseState(warehouseState)
	if err != nil || !warehouseState.Initialized {
		return ErrInvalidSnapshot
	}
	wire := wireRecord{
		SchemaVersion: SchemaVersion, CharacterID: string(record.CharacterID), Revision: record.Revision,
		WorldID: record.Snapshot.World.WorldID, WorldRevision: record.Snapshot.World.Revision, GameplaySHA256: record.Snapshot.World.GameplaySHA256, MapID: record.Snapshot.World.MapID,
		HP: record.Snapshot.HP, MaxHP: record.Snapshot.MaxHP, MP: record.Snapshot.MP, MaxMP: record.Snapshot.MaxMP, Defeated: record.Snapshot.Defeated,
		X: record.Snapshot.Position.X, Y: record.Snapshot.Position.Y, Z: record.Snapshot.Position.Z, Layer: record.Snapshot.Position.Layer, Yaw: record.Snapshot.Yaw,
		Inventory: inventoryState, Warehouse: warehouseState, CombatLoadout: combatLoadoutToWire(record.Snapshot.CombatLoadout), LearnedSkills: learnedSkillsToWire(record.Snapshot.LearnedSkills),
		PrimaryStats: primaryStatsToWire(record.Snapshot.PrimaryStats), SkinID: string(record.Snapshot.SkinID),
	}
	if record.Snapshot.Defeated {
		respawn := record.Snapshot.Respawn
		wire.DefeatedRespawn = &wireDefeatedRespawn{Context: respawn.Context, SpawnPointID: respawn.SpawnPointID, SpawnClass: respawn.SpawnClass, X: respawn.Position.X, Y: respawn.Position.Y, Z: respawn.Position.Z, Layer: respawn.Position.Layer, RemainingTicks: respawn.RemainingTicks, CheckpointID: respawn.CheckpointID}
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
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
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
	return syncDirectory(s.root)
}
