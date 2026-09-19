package characterstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

func encodeSaveJournalRecord(recordID, expectedRevision uint64, intent SaveIntent) ([]byte, error) {
	wire := saveJournalWireRecord{SchemaVersion: SaveJournalSchemaVersion, RecordID: recordID, ExpectedRevision: expectedRevision, IntentID: intent.IntentID, CharacterID: string(intent.Identity.ID), Snapshot: snapshotToSaveJournalWire(intent.Snapshot)}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > maxSaveJournalPayload {
		return nil, fmt.Errorf("%w: encoded payload size=%d", ErrInvalidSnapshot, len(payload))
	}
	return payload, nil
}

func decodeSaveJournalRecord(payload []byte) (uint64, uint64, SaveIntent, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire saveJournalWireRecord
	if err := decoder.Decode(&wire); err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: decode record: %v", ErrCorruptSaveJournal, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: trailing record data", ErrCorruptSaveJournal)
	}
	if (wire.SchemaVersion != LegacySaveJournalSchemaVersion && wire.SchemaVersion != ResourceSaveJournalSchemaVersion && wire.SchemaVersion != InventorySaveJournalSchemaVersion && wire.SchemaVersion != EquipmentSaveJournalSchemaVersion && wire.SchemaVersion != ClassSaveJournalSchemaVersion && wire.SchemaVersion != LoadoutSaveJournalSchemaVersion && wire.SchemaVersion != LearnedSkillsSaveJournalSchemaVersion && wire.SchemaVersion != ClasslessSaveJournalSchemaVersion && wire.SchemaVersion != ItemInstanceSaveJournalSchemaVersion && wire.SchemaVersion != PrimaryStatsSaveJournalSchemaVersion && wire.SchemaVersion != SixPrimaryStatsSaveJournalSchemaVersion && wire.SchemaVersion != AppearanceSaveJournalSchemaVersion && wire.SchemaVersion != MapSaveJournalSchemaVersion && wire.SchemaVersion != WarehouseSaveJournalSchemaVersion) || wire.RecordID == 0 || wire.IntentID == 0 || wire.ExpectedRevision == ^uint64(0) {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: invalid record header", ErrCorruptSaveJournal)
	}
	identity, err := characteridentity.NewTrusted(wire.CharacterID)
	if err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: character identity: %v", ErrCorruptSaveJournal, err)
	}
	snapshot, err := saveJournalWireToSnapshot(wire.SchemaVersion, wire.Snapshot)
	if err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: snapshot: %v", ErrCorruptSaveJournal, err)
	}
	intent := SaveIntent{IntentID: wire.IntentID, Identity: identity, Snapshot: snapshot}
	if err := validateDecodedSaveIntent(wire.SchemaVersion, intent); err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: intent: %v", ErrCorruptSaveJournal, err)
	}
	return wire.RecordID, wire.ExpectedRevision, intent, nil
}

func snapshotToSaveJournalWire(snapshot Snapshot) saveJournalWireSnapshot {
	wire := saveJournalWireSnapshot{
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256, MapID: snapshot.World.MapID,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP, Defeated: snapshot.Defeated,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
		Inventory: snapshot.Inventory, Warehouse: snapshot.Warehouse, CombatLoadout: combatLoadoutToWire(snapshot.CombatLoadout), LearnedSkills: learnedSkillsToWire(snapshot.LearnedSkills),
		PrimaryStats: primaryStatsToWire(snapshot.PrimaryStats), SkinID: string(snapshot.SkinID),
	}
	if snapshot.Defeated {
		respawn := snapshot.Respawn
		wire.DefeatedRespawn = &wireDefeatedRespawn{Context: respawn.Context, SpawnPointID: respawn.SpawnPointID, SpawnClass: respawn.SpawnClass, X: respawn.Position.X, Y: respawn.Position.Y, Z: respawn.Position.Z, Layer: respawn.Position.Layer, RemainingTicks: respawn.RemainingTicks, CheckpointID: respawn.CheckpointID}
	}
	return wire
}

func saveJournalWireToSnapshot(schemaVersion uint16, wire saveJournalWireSnapshot) (Snapshot, error) {
	if schemaVersion < ClassSaveJournalSchemaVersion && wire.ClassID != "" {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= ClasslessSaveJournalSchemaVersion && wire.ClassID != "" {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion < AppearanceSaveJournalSchemaVersion && wire.SkinID != "" {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion < MapSaveJournalSchemaVersion && wire.MapID != "" {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= MapSaveJournalSchemaVersion && !validMapID(wire.MapID) {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion < WarehouseSaveJournalSchemaVersion && (wire.Warehouse.Initialized || len(wire.Warehouse.Items) != 0) {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= WarehouseSaveJournalSchemaVersion && !wire.Warehouse.Initialized {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion < ItemInstanceSaveJournalSchemaVersion && wire.Inventory.HasItemInstances() {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion < PrimaryStatsSaveJournalSchemaVersion && wire.PrimaryStats != nil {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= PrimaryStatsSaveJournalSchemaVersion && wire.PrimaryStats == nil {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion == PrimaryStatsSaveJournalSchemaVersion && hasSixPrimaryWireFields(wire.PrimaryStats) {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= SixPrimaryStatsSaveJournalSchemaVersion && !hasSixPrimaryWireFields(wire.PrimaryStats) {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if schemaVersion >= ClassSaveJournalSchemaVersion && schemaVersion < ClasslessSaveJournalSchemaVersion && wire.ClassID != "" {
		if _, ok := classid.Parse(wire.ClassID); !ok {
			return Snapshot{}, ErrInvalidSnapshot
		}
	}
	mp, maxMP := wire.MP, wire.MaxMP
	if schemaVersion == LegacySaveJournalSchemaVersion {
		mp, maxMP = LegacyDefaultMaxMP, LegacyDefaultMaxMP
	}
	inventoryState := InventoryState{}
	if schemaVersion >= InventorySaveJournalSchemaVersion {
		inventoryState = wire.Inventory
	}
	warehouseState := EmptyWarehouseState()
	if schemaVersion >= WarehouseSaveJournalSchemaVersion {
		var err error
		warehouseState, err = CanonicalWarehouseState(wire.Warehouse)
		if err != nil || !warehouseState.Initialized {
			return Snapshot{}, ErrInvalidSnapshot
		}
	}
	combatLoadout := skillloadout.Slots{}
	if schemaVersion >= LoadoutSaveJournalSchemaVersion {
		var err error
		combatLoadout, err = combatLoadoutFromWire(wire.CombatLoadout)
		if err != nil {
			return Snapshot{}, err
		}
	}
	learnedSkills := learnedskills.Set{}
	if schemaVersion >= LearnedSkillsSaveJournalSchemaVersion {
		var err error
		learnedSkills, err = learnedSkillsFromWire(wire.LearnedSkills)
		if err != nil {
			return Snapshot{}, err
		}
	}
	storeSchema := uint16(0)
	switch {
	case schemaVersion < PrimaryStatsSaveJournalSchemaVersion:
		storeSchema = ItemInstanceSchemaVersion
	case schemaVersion == PrimaryStatsSaveJournalSchemaVersion:
		storeSchema = PrimaryStatsSchemaVersion
	case schemaVersion < AppearanceSaveJournalSchemaVersion:
		storeSchema = SixPrimaryStatsSchemaVersion
	case schemaVersion < MapSaveJournalSchemaVersion:
		storeSchema = AppearanceSchemaVersion
	case schemaVersion < WarehouseSaveJournalSchemaVersion:
		storeSchema = MapSchemaVersion
	default:
		storeSchema = WarehouseSchemaVersion
	}
	primaryStats, err := primaryStatsFromWire(storeSchema, wire.PrimaryStats)
	if err != nil {
		return Snapshot{}, err
	}
	mapID := LegacyDefaultMapID
	if schemaVersion >= MapSaveJournalSchemaVersion {
		mapID = wire.MapID
	}
	skinID := appearance.None
	if schemaVersion >= AppearanceSaveJournalSchemaVersion {
		skinID = appearance.SkinID(wire.SkinID)
		if !appearance.ValidSelection(skinID) {
			return Snapshot{}, ErrInvalidSnapshot
		}
	}
	snapshot := Snapshot{
		World: WorldRef{MapID: mapID, WorldID: wire.WorldID, Revision: wire.WorldRevision, GameplaySHA256: wire.GameplaySHA256},
		HP: wire.HP, MaxHP: wire.MaxHP, MP: mp, MaxMP: maxMP, Defeated: wire.Defeated,
		Position: world.Position{X: wire.X, Y: wire.Y, Z: wire.Z, Layer: wire.Layer}, Yaw: wire.Yaw,
		Inventory: inventoryState, Warehouse: warehouseState, CombatLoadout: combatLoadout, LearnedSkills: learnedSkills, PrimaryStats: primaryStats, SkinID: skinID,
	}
	if wire.DefeatedRespawn != nil {
		snapshot.Respawn = DefeatedRespawn{Context: wire.DefeatedRespawn.Context, SpawnPointID: wire.DefeatedRespawn.SpawnPointID, SpawnClass: wire.DefeatedRespawn.SpawnClass, Position: world.Position{X: wire.DefeatedRespawn.X, Y: wire.DefeatedRespawn.Y, Z: wire.DefeatedRespawn.Z, Layer: wire.DefeatedRespawn.Layer}, RemainingTicks: wire.DefeatedRespawn.RemainingTicks, CheckpointID: wire.DefeatedRespawn.CheckpointID}
	}
	return snapshot, nil
}

func validateNewSaveIntent(intent SaveIntent) error {
	if intent.IntentID == 0 {
		return ErrUnknownSaveIntent
	}
	if err := validateTrustedIdentity(intent.Identity); err != nil {
		return err
	}
	if !intent.Snapshot.Inventory.Initialized || !intent.Snapshot.Warehouse.Initialized {
		return ErrInvalidSnapshot
	}
	return validateSnapshotV15(intent.Snapshot)
}

func validateDecodedSaveIntent(schemaVersion uint16, intent SaveIntent) error {
	if intent.IntentID == 0 {
		return ErrUnknownSaveIntent
	}
	if err := validateTrustedIdentity(intent.Identity); err != nil {
		return err
	}
	if schemaVersion < ItemInstanceSaveJournalSchemaVersion && intent.Snapshot.Inventory.HasItemInstances() {
		return ErrInvalidSnapshot
	}
	if err := validateSnapshotV5(intent.Snapshot); err != nil {
		return err
	}
	switch schemaVersion {
	case LegacySaveJournalSchemaVersion, ResourceSaveJournalSchemaVersion:
		if intent.Snapshot.Inventory != (InventoryState{}) || intent.Snapshot.CombatLoadout != (skillloadout.Slots{}) || intent.Snapshot.LearnedSkills != (learnedskills.Set{}) {
			return ErrInvalidSnapshot
		}
	case InventorySaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized || intent.Snapshot.Inventory.OffHand != "" || intent.Snapshot.CombatLoadout != (skillloadout.Slots{}) || intent.Snapshot.LearnedSkills != (learnedskills.Set{}) {
			return ErrInvalidSnapshot
		}
	case EquipmentSaveJournalSchemaVersion, ClassSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized || intent.Snapshot.CombatLoadout != (skillloadout.Slots{}) || intent.Snapshot.LearnedSkills != (learnedskills.Set{}) {
			return ErrInvalidSnapshot
		}
		if schemaVersion == ClassSaveJournalSchemaVersion {
			if err := validateSnapshotV6(intent.Snapshot); err != nil {
				return err
			}
		}
	case LoadoutSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized || intent.Snapshot.LearnedSkills != (learnedskills.Set{}) {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV7(intent.Snapshot); err != nil {
			return err
		}
	case LearnedSkillsSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV8(intent.Snapshot); err != nil {
			return err
		}
	case ClasslessSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV9(intent.Snapshot); err != nil {
			return err
		}
	case ItemInstanceSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV10(intent.Snapshot); err != nil {
			return err
		}
	case PrimaryStatsSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV11(intent.Snapshot); err != nil {
			return err
		}
	case SixPrimaryStatsSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV12(intent.Snapshot); err != nil {
			return err
		}
	case AppearanceSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV13(intent.Snapshot); err != nil {
			return err
		}
	case MapSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV14(intent.Snapshot); err != nil {
			return err
		}
	case WarehouseSaveJournalSchemaVersion:
		if !intent.Snapshot.Inventory.Initialized || !intent.Snapshot.Warehouse.Initialized {
			return ErrInvalidSnapshot
		}
		if err := validateSnapshotV15(intent.Snapshot); err != nil {
			return err
		}
	default:
		return ErrInvalidSnapshot
	}
	return nil
}
