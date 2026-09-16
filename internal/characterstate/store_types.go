package characterstate

import (
	"errors"
	"sync"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	LegacySchemaVersion          uint16 = 1
	RespawnSchemaVersion         uint16 = 2
	ResourceSchemaVersion        uint16 = 3
	InventorySchemaVersion       uint16 = 4
	EquipmentSchemaVersion       uint16 = 5
	ClassSchemaVersion           uint16 = 6
	LoadoutSchemaVersion         uint16 = 7
	LearnedSkillsSchemaVersion   uint16 = 8
	ClasslessSchemaVersion       uint16 = 9
	ItemInstanceSchemaVersion    uint16 = 10
	PrimaryStatsSchemaVersion    uint16 = 11 // historical two-attribute interim schema
	SixPrimaryStatsSchemaVersion uint16 = 12
	AppearanceSchemaVersion      uint16 = 13
	MapSchemaVersion             uint16 = 14
	SchemaVersion                uint16 = MapSchemaVersion
	LegacyDefaultMaxMP           uint32 = 100
	LegacyDefaultMapID                  = "map1"
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
	MapID          string
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

// Snapshot is the current durable character-state contract. Profession/ClassID is deliberately
// absent. Schema v11 was a short-lived two-attribute Strength/Agility foundation, schema v12 owns
// all six formal classless base attributes, schema v13 adds the Server-owned selected SkinID, and
// schema v14 adds the Server-owned MapID. Allocation provenance/level points remain separate future
// state until a formal character-level owner exists.
type Snapshot struct {
	World         WorldRef
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
	PrimaryStats  characterstats.Primary
	SkinID        appearance.SkinID
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

type wirePrimaryStats struct {
	Strength     uint32  `json:"strength"`
	Agility      uint32  `json:"agility"`
	Constitution *uint32 `json:"constitution,omitempty"`
	Intelligence *uint32 `json:"intelligence,omitempty"`
	Spirit       *uint32 `json:"spirit,omitempty"`
	Charisma     *uint32 `json:"charisma,omitempty"`
}

type wireRecord struct {
	SchemaVersion   uint16               `json:"schema_version"`
	CharacterID     string               `json:"character_id"`
	Revision        uint64               `json:"revision"`
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
	CombatLoadout   []string             `json:"combat_loadout,omitempty"`
	LearnedSkills   []string             `json:"learned_skills,omitempty"`
	PrimaryStats    *wirePrimaryStats    `json:"primary_stats,omitempty"`
}
