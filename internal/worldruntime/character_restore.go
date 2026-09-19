package worldruntime

import (
	"errors"
	"math"
	"strings"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterRestoreRequiresTrustedIdentity  = errors.New("worldruntime: character restore requires trusted identity")
	ErrCharacterRestoreIdentityMismatch         = errors.New("worldruntime: character restore identity mismatch")
	ErrCharacterRestoreWorldMismatch            = errors.New("worldruntime: character restore gameplay world mismatch")
	ErrCharacterRestoreMapMismatch              = errors.New("worldruntime: character restore map mismatch")
	ErrCharacterRestoreInvalid                  = errors.New("worldruntime: invalid character restore")
	ErrCharacterRestoreDefeatedUnsupported      = errors.New("worldruntime: legacy defeated character restore is not supported")
	ErrCharacterRestoreRespawnPolicyUnavailable = errors.New("worldruntime: defeated character restore requires respawn policy")
)

type CharacterRestore struct {
	SchemaVersion uint16
	CharacterID   characteridentity.ID
	Revision      uint64
	World         protocol.WorldIdentity
	MapID         gameplayworld.MapID
	HP            uint32
	MaxHP         uint32
	MP            uint32
	MaxMP         uint32
	Defeated      bool
	Transform     world.Transform
	Respawn       characterstate.DefeatedRespawn
	Inventory     characterstate.InventoryState
	Warehouse     characterstate.WarehouseState
	CombatLoadout skillloadout.Slots
	LearnedSkills learnedskills.Set
	PrimaryStats  characterstats.Primary
	SkinID        appearance.SkinID
}

func CharacterRestoreFromRecord(record characterstate.Record) CharacterRestore {
	return CharacterRestore{
		SchemaVersion: record.SchemaVersion,
		CharacterID:   record.CharacterID,
		Revision:      record.Revision,
		World:         protocol.WorldIdentity{WorldID: record.Snapshot.World.WorldID, Revision: record.Snapshot.World.Revision, GameplaySHA256: record.Snapshot.World.GameplaySHA256},
		MapID:         gameplayworld.MapID(record.Snapshot.World.MapID),
		HP:            record.Snapshot.HP,
		MaxHP:         record.Snapshot.MaxHP,
		MP:            record.Snapshot.MP,
		MaxMP:         record.Snapshot.MaxMP,
		Defeated:      record.Snapshot.Defeated,
		Transform:     world.Transform{Position: record.Snapshot.Position, Yaw: record.Snapshot.Yaw},
		Respawn:       record.Snapshot.Respawn,
		Inventory:     record.Snapshot.Inventory,
		Warehouse:     record.Snapshot.Warehouse,
		CombatLoadout: record.Snapshot.CombatLoadout,
		LearnedSkills: record.Snapshot.LearnedSkills,
		PrimaryStats:  record.Snapshot.PrimaryStats,
		SkinID:        record.Snapshot.SkinID,
	}
}

func resolvedRestoreMapID(restore CharacterRestore, currentWorld protocol.WorldIdentity) (gameplayworld.MapID, bool) {
	raw := string(restore.MapID)
	if raw != strings.TrimSpace(raw) {
		return "", false
	}
	if raw != "" {
		return restore.MapID, true
	}
	// Server-internal bootstraps that predate explicit MapID are ordinary player
	// characters and therefore default only to map1. map0/gm-room is never implicit.
	if currentWorld.WorldID != "gm-room" {
		return gameplayworld.MapIDStarterVillage, true
	}
	return "", false
}

func ValidateCharacterRestore(identity characteridentity.Binding, restore CharacterRestore, currentWorld protocol.WorldIdentity) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrCharacterRestoreRequiresTrustedIdentity
	}
	if restore.CharacterID != identity.ID {
		return ErrCharacterRestoreIdentityMismatch
	}
	if restore.Revision == 0 || !restore.World.Valid() || !currentWorld.Valid() {
		return ErrCharacterRestoreInvalid
	}
	if restore.World != currentWorld {
		return ErrCharacterRestoreWorldMismatch
	}
	if _, ok := resolvedRestoreMapID(restore, currentWorld); !ok {
		return ErrCharacterRestoreInvalid
	}
	if restore.MaxHP == 0 || restore.HP > restore.MaxHP || restore.MaxMP == 0 || restore.MP > restore.MaxMP {
		return ErrCharacterRestoreInvalid
	}
	if err := validateCharacterPrimaryStatsRestore(restore.SchemaVersion, restore.PrimaryStats); err != nil {
		return err
	}
	if restore.SchemaVersion < characterstate.AppearanceSchemaVersion {
		if restore.SkinID != appearance.None {
			return ErrCharacterRestoreInvalid
		}
	} else if !appearance.ValidSelection(restore.SkinID) {
		return ErrCharacterRestoreInvalid
	}
	if restore.SchemaVersion < characterstate.InventorySchemaVersion && restore.Inventory != (characterstate.InventoryState{}) {
		return ErrCharacterRestoreInvalid
	}
	if restore.SchemaVersion < characterstate.ItemInstanceSchemaVersion && restore.Inventory.HasItemInstances() {
		return ErrCharacterRestoreInvalid
	}
	if restore.Inventory.Initialized {
		canonical, err := characterstate.CanonicalInventoryState(restore.Inventory)
		if err != nil || canonical != restore.Inventory {
			return ErrCharacterRestoreInvalid
		}
		if err := validateRestoredEquipmentBaseRequirements(restore.Inventory, restore.PrimaryStats); err != nil {
			return err
		}
	} else if restore.Inventory != (characterstate.InventoryState{}) {
		return ErrCharacterRestoreInvalid
	}
	if restore.SchemaVersion < characterstate.WarehouseSchemaVersion {
		if restore.Warehouse.Initialized || len(restore.Warehouse.Items) != 0 {
			return ErrCharacterRestoreInvalid
		}
	} else {
		if !restore.Warehouse.Initialized {
			return ErrCharacterRestoreInvalid
		}
		if _, ok := canonicalWarehouseState(restore.Warehouse); !ok {
			return ErrCharacterRestoreInvalid
		}
	}
	if err := validateCharacterSkillRestore(restore.SchemaVersion, restore.LearnedSkills, restore.CombatLoadout); err != nil {
		return err
	}
	for _, value := range []float32{restore.Transform.Position.X, restore.Transform.Position.Y, restore.Transform.Position.Z, restore.Transform.Yaw} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return ErrCharacterRestoreInvalid
		}
	}
	if restore.Defeated {
		if restore.SchemaVersion < characterstate.RespawnSchemaVersion {
			return ErrCharacterRestoreDefeatedUnsupported
		}
		if restore.HP != 0 || !validRestoreDeathContext(restore.Respawn.Context) || restore.Respawn.SpawnPointID == "" || !validRestoreSpawnClass(restore.Respawn.SpawnClass) {
			return ErrCharacterRestoreInvalid
		}
		for _, value := range []float32{restore.Respawn.Position.X, restore.Respawn.Position.Y, restore.Respawn.Position.Z} {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return ErrCharacterRestoreInvalid
			}
		}
		return nil
	}
	if restore.HP == 0 || restore.Respawn != (characterstate.DefeatedRespawn{}) {
		return ErrCharacterRestoreInvalid
	}
	return nil
}

func validateCharacterPrimaryStatsRestore(schemaVersion uint16, primary characterstats.Primary) error {
	if err := characterstats.ValidateBase(primary); err != nil {
		return ErrCharacterRestoreInvalid
	}
	neutral := characterstats.DefaultPrimary()
	if schemaVersion < characterstate.PrimaryStatsSchemaVersion {
		if primary != neutral {
			return ErrCharacterRestoreInvalid
		}
		return nil
	}
	if schemaVersion == characterstate.PrimaryStatsSchemaVersion {
		if primary.Constitution != neutral.Constitution || primary.Intelligence != neutral.Intelligence || primary.Spirit != neutral.Spirit || primary.Charisma != neutral.Charisma {
			return ErrCharacterRestoreInvalid
		}
	}
	return nil
}

func validRestoreDeathContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege:
		return true
	default:
		return false
	}
}

func validRestoreSpawnClass(class respawnpolicy.SpawnClass) bool {
	switch class {
	case respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint, respawnpolicy.SpawnClassSiege:
		return true
	default:
		return false
	}
}

func (r *Runtime) validateCharacterRestore(s *session.Session, restore CharacterRestore) error {
	if s == nil {
		return session.ErrInvalidSession
	}
	currentWorld := protocol.WorldIdentity{WorldID: r.characterStateWorld.WorldID, Revision: r.characterStateWorld.Revision, GameplaySHA256: r.characterStateWorld.GameplaySHA256}
	if err := ValidateCharacterRestore(s.CharacterIdentity, restore, currentWorld); err != nil {
		return err
	}
	mapID, ok := resolvedRestoreMapID(restore, currentWorld)
	if !ok || string(mapID) != r.characterStateWorld.MapID {
		return ErrCharacterRestoreMapMismatch
	}
	return nil
}
