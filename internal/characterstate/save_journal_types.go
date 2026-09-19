package characterstate

import (
	"errors"
	"hash/crc32"
	"os"
	"sync"

	"github.com/li41/astrahold-server/internal/world"
)

const (
	LegacySaveJournalSchemaVersion          uint16 = 1
	ResourceSaveJournalSchemaVersion        uint16 = 2
	InventorySaveJournalSchemaVersion       uint16 = 3
	EquipmentSaveJournalSchemaVersion       uint16 = 4
	ClassSaveJournalSchemaVersion           uint16 = 5
	LoadoutSaveJournalSchemaVersion         uint16 = 6
	LearnedSkillsSaveJournalSchemaVersion   uint16 = 7
	ClasslessSaveJournalSchemaVersion       uint16 = 8
	ItemInstanceSaveJournalSchemaVersion    uint16 = 9
	PrimaryStatsSaveJournalSchemaVersion    uint16 = 10 // historical two-attribute interim schema
	SixPrimaryStatsSaveJournalSchemaVersion uint16 = 11
	AppearanceSaveJournalSchemaVersion      uint16 = 12
	MapSaveJournalSchemaVersion             uint16 = 13
	WarehouseSaveJournalSchemaVersion       uint16 = 14
	SaveJournalSchemaVersion                uint16 = WarehouseSaveJournalSchemaVersion
	saveCheckpointSchemaVersion             uint16 = 1
	saveJournalIDSize                              = 16
	maxSaveJournalPayload                          = 1 << 20
)

var (
	saveJournalMagic = []byte("ASTRAHOLD-CHARACTER-STATE-SAVE-JOURNAL-V1\n")
	saveCRCTable     = crc32.MakeTable(crc32.Castagnoli)
)

var (
	ErrInvalidSaveJournalPath        = errors.New("characterstate: invalid save journal path")
	ErrCorruptSaveJournal            = errors.New("characterstate: corrupt save journal")
	ErrSaveJournalClosed             = errors.New("characterstate: save journal closed")
	ErrSaveJournalRecordOverflow     = errors.New("characterstate: save journal record id overflow")
	ErrInvalidSaveCheckpointPath     = errors.New("characterstate: invalid save checkpoint path")
	ErrCorruptSaveCheckpoint         = errors.New("characterstate: corrupt save checkpoint")
	ErrSaveCheckpointJournalMismatch = errors.New("characterstate: save checkpoint journal mismatch")
	ErrSaveCheckpointAhead           = errors.New("characterstate: save checkpoint ahead of journal")
	ErrSaveCheckpointOffsetMismatch  = errors.New("characterstate: save checkpoint offset mismatch")
)

type SaveJournalRecord struct {
	RecordID         uint64
	ExpectedRevision uint64
	Intent           SaveIntent
	EndOffset        int64
}

type SaveCheckpoint struct {
	JournalID string
	RecordID  uint64
	Offset    int64
}

type SaveJournal struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	journalID    [saveJournalIDSize]byte
	journalIDHex string
	lastRecordID uint64
	endOffset    int64
	recordEnds   []int64
	repairedTail bool
	closed       bool
}

type SaveCheckpointStore struct{ path string }

type saveJournalWireRecord struct {
	SchemaVersion    uint16                  `json:"schema_version"`
	RecordID         uint64                  `json:"record_id"`
	ExpectedRevision uint64                  `json:"expected_revision"`
	IntentID         uint64                  `json:"intent_id"`
	CharacterID      string                  `json:"character_id"`
	Snapshot         saveJournalWireSnapshot `json:"snapshot"`
}

type saveJournalWireSnapshot struct {
	WorldID         string               `json:"world_id"`
	WorldRevision   string               `json:"world_revision"`
	GameplaySHA256  string               `json:"gameplay_sha256"`
	MapID           string               `json:"map_id,omitempty"`
	ClassID         string               `json:"class_id,omitempty"`
	SkinID          string               `json:"skin_id,omitempty"`
	HP              uint32               `json:"hp"`
	MaxHP           uint32               `json:"max_hp"`
	MP              uint32               `json:"mp,omitempty"`
	MaxMP           uint32               `json:"max_mp,omitempty"`
	Defeated        bool                 `json:"defeated"`
	X               float32              `json:"x"`
	Y               float32              `json:"y"`
	Z               float32              `json:"z"`
	Layer           world.LayerID        `json:"layer"`
	Yaw             float32              `json:"yaw"`
	DefeatedRespawn *wireDefeatedRespawn `json:"defeated_respawn,omitempty"`
	Inventory       InventoryState       `json:"inventory,omitempty"`
	Warehouse       WarehouseState       `json:"warehouse,omitempty"`
	CombatLoadout   []string             `json:"combat_loadout,omitempty"`
	LearnedSkills   []string             `json:"learned_skills,omitempty"`
	PrimaryStats    *wirePrimaryStats    `json:"primary_stats,omitempty"`
}

type saveCheckpointWire struct {
	SchemaVersion uint16 `json:"schema_version"`
	JournalID     string `json:"journal_id"`
	RecordID      uint64 `json:"record_id"`
	Offset        int64  `json:"offset"`
}
